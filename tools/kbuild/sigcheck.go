// SPDX-License-Identifier: BSD-2-Clause

package main

// Signature checks of sources: OpenPGP (detached signatures and signed
// messages) and minisign. kbuild does them itself, so that the downloads
// need no gpgv or minisign program. Read docs/sources-and-keys.md.

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/clearsign"
	pgperrors "github.com/ProtonMail/go-crypto/openpgp/errors"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"golang.org/x/crypto/blake2b"
)

const armorKeyBlock = "-----BEGIN PGP PUBLIC KEY BLOCK-----"

// readKeyring reads the OpenPGP keys keys/<name>.asc. A file can have more
// than one armored key block.
func readKeyring(names []string) (openpgp.EntityList, error) {
	var ring openpgp.EntityList
	for _, n := range names {
		p := filepath.Join(KEYS, n+".asc")
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		blocks := strings.Split(string(b), armorKeyBlock)
		for _, blk := range blocks[1:] {
			el, err := openpgp.ReadArmoredKeyRing(strings.NewReader(armorKeyBlock + blk))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			ring = append(ring, el...)
		}
		if len(blocks) < 2 {
			return nil, fmt.Errorf("%s: no armored public key", p)
		}
	}
	return ring, nil
}

// pgpPackets returns the binary OpenPGP data of a file that can be armored.
func pgpPackets(b []byte) ([]byte, error) {
	if !bytes.Contains(b[:min(len(b), 1024)], []byte("-----BEGIN PGP")) {
		return b, nil
	}
	blk, err := armor.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(blk.Body)
}

// sigConfig checks the keys at the time of the signature: a key that
// expired after the release still verifies the release (as with gpgv).
func sigConfig(created time.Time) *packet.Config {
	return &packet.Config{Time: func() time.Time { return created }}
}

// expiredErr tells if an error is only an expiry of the key or the
// signature, so that a check at the time of the signature can follow.
func expiredErr(err error) bool {
	return errors.Is(err, pgperrors.ErrKeyExpired) || errors.Is(err, pgperrors.ErrSignatureExpired)
}

// verifyDetached checks a detached OpenPGP signature of a file. With
// allowExpired, a key that expired before the signature is accepted (a
// recipe sets allow-expired-key when upstream signs with an expired key).
func verifyDetached(ring openpgp.EntityList, sigPath, dataPath string, allowExpired bool) error {
	signer, err := verifyDetachedSigner(ring, sigPath, dataPath, allowExpired)
	if err != nil {
		return err
	}
	logf("signature by %s", signerName(signer))
	return nil
}

// verifyDetachedSigner is verifyDetached; it returns the key that made the
// signature.
func verifyDetachedSigner(ring openpgp.EntityList, sigPath, dataPath string, allowExpired bool) (*openpgp.Entity, error) {
	raw, err := os.ReadFile(sigPath)
	if err != nil {
		return nil, err
	}
	sigData, err := pgpPackets(raw)
	if err != nil {
		return nil, fmt.Errorf("signature: %w", err)
	}
	p, err := packet.Read(bytes.NewReader(sigData))
	if err != nil {
		return nil, fmt.Errorf("signature: %w", err)
	}
	sig, ok := p.(*packet.Signature)
	if !ok {
		return nil, errors.New("the signature file has no signature packet")
	}
	data, err := os.Open(dataPath)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(data)
	// First check at the current time (the newest self-signatures of the
	// key apply), then at the time of the signature.
	signer, err := openpgp.CheckDetachedSignature(ring, data, bytes.NewReader(sigData), nil)
	if err != nil && expiredErr(err) {
		if _, serr := data.Seek(0, io.SeekStart); serr != nil {
			return nil, serr
		}
		signer, err = openpgp.CheckDetachedSignature(ring, data, bytes.NewReader(sigData), sigConfig(sig.CreationTime))
	}
	if err != nil && expiredErr(err) && allowExpired {
		if _, serr := data.Seek(0, io.SeekStart); serr != nil {
			return nil, serr
		}
		signer, err = verifyIgnoringExpiry(ring, sig, data)
		if err == nil {
			logf("warning: the key %s has expired; the recipe accepts it (allow-expired-key)", signerName(signer))
		}
	}
	if err != nil {
		return nil, err
	}
	return signer, nil
}

// verifyMessage checks a signed OpenPGP message (also a cleartext signed
// message) and returns its text.
func verifyMessage(ring openpgp.EntityList, msgPath string) (string, error) {
	raw, err := os.ReadFile(msgPath)
	if err != nil {
		return "", err
	}
	if bytes.Contains(raw[:min(len(raw), 1024)], []byte("-----BEGIN PGP SIGNED MESSAGE-----")) {
		return verifyClearsigned(ring, raw)
	}
	data, err := pgpPackets(raw)
	if err != nil {
		return "", err
	}
	read := func(cfg *packet.Config) (*openpgp.MessageDetails, []byte, error) {
		md, err := openpgp.ReadMessage(bytes.NewReader(data), ring, nil, cfg)
		if err != nil {
			return nil, nil, err
		}
		text, err := io.ReadAll(md.UnverifiedBody)
		return md, text, err
	}
	// First check at the current time, then at the time of the signature.
	md, text, err := read(nil)
	if err != nil {
		return "", err
	}
	if md.SignatureError != nil && expiredErr(md.SignatureError) && md.Signature != nil {
		if md, text, err = read(sigConfig(md.Signature.CreationTime)); err != nil {
			return "", err
		}
	}
	if !md.IsSigned || md.SignedBy == nil {
		return "", errors.New("the message is not signed by a known key")
	}
	if md.SignatureError != nil {
		return "", md.SignatureError
	}
	logf("signature by %s", signerName(md.SignedBy.Entity))
	return string(text), nil
}

// verifyClearsigned checks a cleartext signed message. As for a detached
// signature, the check is first at the current time, then at the time of
// the signature.
func verifyClearsigned(ring openpgp.EntityList, raw []byte) (string, error) {
	blk, _ := clearsign.Decode(raw)
	if blk == nil || blk.ArmoredSignature == nil {
		return "", errors.New("not a cleartext signed message")
	}
	sigData, err := io.ReadAll(blk.ArmoredSignature.Body)
	if err != nil {
		return "", err
	}
	signer, err := openpgp.CheckDetachedSignature(ring, bytes.NewReader(blk.Bytes), bytes.NewReader(sigData), nil)
	if err != nil && expiredErr(err) {
		p, perr := packet.Read(bytes.NewReader(sigData))
		sig, ok := p.(*packet.Signature)
		if perr != nil || !ok {
			return "", err
		}
		signer, err = openpgp.CheckDetachedSignature(ring, bytes.NewReader(blk.Bytes), bytes.NewReader(sigData),
			sigConfig(sig.CreationTime))
	}
	if err != nil {
		return "", err
	}
	logf("signature by %s", signerName(signer))
	return string(blk.Plaintext), nil
}

func signerName(e *openpgp.Entity) string {
	if e == nil {
		return "an unknown key"
	}
	if id := e.PrimaryIdentity(); id != nil {
		return fmt.Sprintf("%q (%X)", id.Name, e.PrimaryKey.Fingerprint)
	}
	return fmt.Sprintf("%X", e.PrimaryKey.Fingerprint)
}

// minisign files: a public key is "untrusted comment" and base64 of
// "Ed" + key ID (8) + Ed25519 key (32). A signature is "untrusted comment",
// base64 of algorithm (2) + key ID (8) + signature (64), "trusted comment"
// and base64 of the global signature (of the signature and the trusted
// comment). Algorithm "ED" signs the BLAKE2b-512 hash of the file.

func minisignLines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// verifyMinisign checks a minisign signature of a file.
func verifyMinisign(pubPath, sigPath, dataPath string) error {
	pl, err := minisignLines(pubPath)
	if err != nil {
		return err
	}
	if len(pl) < 2 {
		return fmt.Errorf("%s: not a minisign public key", pubPath)
	}
	pk, err := base64.StdEncoding.DecodeString(pl[1])
	if err != nil || len(pk) != 42 || string(pk[:2]) != "Ed" {
		return fmt.Errorf("%s: not a minisign public key", pubPath)
	}
	sl, err := minisignLines(sigPath)
	if err != nil {
		return err
	}
	if len(sl) != 4 || !strings.HasPrefix(sl[2], "trusted comment: ") {
		return errors.New("not a minisign signature")
	}
	sig, err := base64.StdEncoding.DecodeString(sl[1])
	if err != nil || len(sig) != 74 {
		return errors.New("bad minisign signature")
	}
	if !bytes.Equal(sig[2:10], pk[2:10]) {
		return errors.New("the minisign signature has a different key ID")
	}
	data, err := os.ReadFile(dataPath)
	if err != nil {
		return err
	}
	msg := data
	switch string(sig[:2]) {
	case "Ed":
	case "ED":
		h := blake2b.Sum512(data)
		msg = h[:]
	default:
		return fmt.Errorf("unknown minisign algorithm %q", sig[:2])
	}
	key := ed25519.PublicKey(pk[10:])
	if !ed25519.Verify(key, msg, sig[10:]) {
		return errors.New("the minisign signature is not valid")
	}
	global, err := base64.StdEncoding.DecodeString(sl[3])
	if err != nil {
		return errors.New("bad minisign global signature")
	}
	trusted := strings.TrimPrefix(sl[2], "trusted comment: ")
	if !ed25519.Verify(key, append(append([]byte{}, sig[10:]...), trusted...), global) {
		return errors.New("the trusted comment of the minisign signature is not valid")
	}
	return nil
}

// verifyIgnoringExpiry checks a signature with the key that has its key ID,
// also if the key has expired. A revoked key is not accepted.
func verifyIgnoringExpiry(ring openpgp.EntityList, sig *packet.Signature, data io.Reader) (*openpgp.Entity, error) {
	if sig.IssuerKeyId == nil {
		return nil, errors.New("the signature has no issuer key ID")
	}
	for _, e := range ring {
		if len(e.Revocations) > 0 {
			continue
		}
		keys := []*packet.PublicKey{e.PrimaryKey}
		for _, sk := range e.Subkeys {
			if len(sk.Revocations) == 0 {
				keys = append(keys, sk.PublicKey)
			}
		}
		for _, k := range keys {
			if k.KeyId != *sig.IssuerKeyId {
				continue
			}
			h, err := sig.PrepareVerify()
			if err != nil {
				return nil, err
			}
			if _, err := io.Copy(h, data); err != nil {
				return nil, err
			}
			if err := k.VerifySignature(h, sig); err != nil {
				return nil, err
			}
			return e, nil
		}
	}
	return nil, errors.New("signature made by unknown entity")
}
