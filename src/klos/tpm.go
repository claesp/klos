// SPDX-License-Identifier: BSD-2-Clause

package main

// TPM2 unlock of the LUKS2 disk. Read docs/tpm.md.
//
// "klos tpm enroll" makes a random disk key and seals it in the TPM with a
// policy for two PCRs (SHA-256 bank):
//
//	PCR 7   the Secure Boot state and keys (PK, KEK, db, dbx) and the
//	        certificate that verified the EFI binary
//	PCR 15  zero (its value at the start of each boot)
//
// The key is a new LUKS2 keyslot. The sealed object is a LUKS2 token of the
// type "klos-tpm2" in the disk header. /init unseals the key, unlocks the
// disk and then always extends PCR 15, so that nobody can unseal the key
// again before the next boot. The sessions are salted with the storage root
// key (SRK) and encrypt the key, and /init compares the SRK name with the
// enrolled name, so that a different TPM cannot answer.
//
// With a PIN ("klos tpm enroll -pin"), the policy also has PolicyAuthValue:
// the TPM gives the key only with the PIN. The object then does not have
// the "no dictionary attack" attribute, so the TPM lockout limits the PIN
// guesses.

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpm2/transport"
	"github.com/google/go-tpm/tpm2/transport/linuxtpm"
)

const (
	tpmDevice   = "/dev/tpmrm0"
	tpmToken    = "klos-tpm2"
	pcrSecBoot  = 7
	pcrLock     = 15
	keyBytes    = 32
	lockMessage = "klos: disk unlocked"
)

var (
	errNoToken  = errors.New("no TPM key is enrolled")
	errWrongPIN = errors.New("the TPM PIN is not correct")
	errLockout  = errors.New("the TPM is in lockout after too many wrong PINs")
	errSkipped  = errors.New("no TPM PIN")
)

const minPINLength = 6

// pinAuth is the TPM auth value of a PIN (a SHA-256 digest, the size of
// the name algorithm).
func pinAuth(pin string) []byte {
	d := sha256.Sum256([]byte("klos-tpm-pin\x00" + pin))
	return d[:]
}

// tpmTokenData is the JSON of the LUKS2 token.
type tpmTokenData struct {
	Type     string   `json:"type"`
	Keyslots []string `json:"keyslots"`
	Version  int      `json:"klos_version"`
	SRKName  string   `json:"tpm2_srk_name"`
	Public   string   `json:"tpm2_public"`
	Private  string   `json:"tpm2_private"`
	PCRs     []int    `json:"tpm2_pcrs"`
	PCR7     string   `json:"tpm2_pcr7"`
	PIN      bool     `json:"tpm2_pin"`
}

type luksMetadata struct {
	Keyslots map[string]json.RawMessage `json:"keyslots"`
	Tokens   map[string]json.RawMessage `json:"tokens"`
}

func luksDump(dev string) (*luksMetadata, error) {
	out, err := exec.Command("cryptsetup", "luksDump", "--dump-json-metadata", dev).Output()
	if err != nil {
		return nil, fmt.Errorf("cryptsetup luksDump: %w", err)
	}
	m := &luksMetadata{}
	if err := json.Unmarshal(out, m); err != nil {
		return nil, err
	}
	return m, nil
}

// findTPMToken returns the token ID and data of the klos-tpm2 token.
func findTPMToken(dev string) (string, *tpmTokenData, error) {
	m, err := luksDump(dev)
	if err != nil {
		return "", nil, err
	}
	for id, raw := range m.Tokens {
		t := &tpmTokenData{}
		if json.Unmarshal(raw, t) == nil && t.Type == tpmToken {
			return id, t, nil
		}
	}
	return "", nil, errNoToken
}

func openTPM(wait time.Duration) (transport.TPMCloser, error) {
	end := time.Now().Add(wait)
	for {
		if _, err := os.Stat(tpmDevice); err == nil {
			return linuxtpm.Open(tpmDevice)
		}
		if time.Now().After(end) {
			return nil, fmt.Errorf("no TPM (%s)", tpmDevice)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// srk makes the storage root key (TCG ECC P-256 template). The TPM makes
// the same key each time from its owner seed.
func srk(t transport.TPM) (tpm2.NamedHandle, *tpm2.TPMTPublic, error) {
	rsp, err := tpm2.CreatePrimary{
		PrimaryHandle: tpm2.TPMRHOwner,
		InPublic:      tpm2.New2B(tpm2.ECCSRKTemplate),
	}.Execute(t)
	if err != nil {
		return tpm2.NamedHandle{}, nil, fmt.Errorf("create the SRK: %w", err)
	}
	pub, err := rsp.OutPublic.Contents()
	if err != nil {
		return tpm2.NamedHandle{}, nil, err
	}
	return tpm2.NamedHandle{Handle: rsp.ObjectHandle, Name: rsp.Name}, pub, nil
}

func flush(t transport.TPM, h tpm2.TPMHandle) {
	// The TPM also flushes transient objects at the next boot, so a flush
	// error has no lasting effect.
	_, _ = tpm2.FlushContext{FlushHandle: h}.Execute(t)
}

func pcrSelection(pcrs ...uint) tpm2.TPMLPCRSelection {
	return tpm2.TPMLPCRSelection{PCRSelections: []tpm2.TPMSPCRSelection{{
		Hash:      tpm2.TPMAlgSHA256,
		PCRSelect: tpm2.PCClientCompatible.PCRs(pcrs...),
	}}}
}

func readPCR(t transport.TPM, pcr uint) ([]byte, error) {
	rsp, err := tpm2.PCRRead{PCRSelectionIn: pcrSelection(pcr)}.Execute(t)
	if err != nil {
		return nil, err
	}
	if len(rsp.PCRValues.Digests) != 1 {
		return nil, fmt.Errorf("PCR %d is not in the SHA-256 bank", pcr)
	}
	return rsp.PCRValues.Digests[0].Buffer, nil
}

// pcrPolicy is the policy digest for PolicyPCR with PCR 7 = pcr7 and
// PCR 15 = zero, and PolicyAuthValue if the key has a PIN.
func pcrPolicy(pcr7 []byte, pin bool) ([]byte, error) {
	h := sha256.New()
	h.Write(pcr7)
	h.Write(make([]byte, sha256.Size))
	calc, err := tpm2.NewPolicyCalculator(tpm2.TPMAlgSHA256)
	if err != nil {
		return nil, err
	}
	cmd := tpm2.PolicyPCR{
		PcrDigest: tpm2.TPM2BDigest{Buffer: h.Sum(nil)},
		Pcrs:      pcrSelection(pcrSecBoot, pcrLock),
	}
	if err := cmd.Update(calc); err != nil {
		return nil, err
	}
	if pin {
		if err := (tpm2.PolicyAuthValue{}).Update(calc); err != nil {
			return nil, err
		}
	}
	return calc.Hash().Digest, nil
}

// lockPCR extends PCR 15. After this, the TPM does not unseal the disk key
// before the next boot. A computer without a TPM is not an error.
func lockPCR() error {
	if _, err := os.Stat(tpmDevice); err != nil {
		return nil
	}
	t, err := openTPM(0)
	if err != nil {
		return err
	}
	defer closeQuietly(t)
	d := sha256.Sum256([]byte(lockMessage))
	_, err = tpm2.PCRExtend{
		PCRHandle: tpm2.AuthHandle{Handle: tpm2.TPMHandle(pcrLock), Auth: tpm2.PasswordAuth(nil)},
		Digests:   tpm2.TPMLDigestValues{Digests: []tpm2.TPMTHA{{HashAlg: tpm2.TPMAlgSHA256, Digest: d[:]}}},
	}.Execute(t)
	if err != nil {
		return fmt.Errorf("extend PCR %d: %w", pcrLock, err)
	}
	return nil
}

// tpmUnseal unseals the disk key of a token. pin is used only if the token
// has a PIN.
func tpmUnseal(tok *tpmTokenData, pin string) ([]byte, error) {
	pubB, err1 := base64.StdEncoding.DecodeString(tok.Public)
	privB, err2 := base64.StdEncoding.DecodeString(tok.Private)
	if err1 != nil || err2 != nil {
		return nil, errors.New("the TPM token is damaged")
	}
	pub, err := tpm2.Unmarshal[tpm2.TPM2BPublic](pubB)
	if err != nil {
		return nil, err
	}
	priv, err := tpm2.Unmarshal[tpm2.TPM2BPrivate](privB)
	if err != nil {
		return nil, err
	}
	t, err := openTPM(3 * time.Second)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(t)
	parent, parentPub, err := srk(t)
	if err != nil {
		return nil, err
	}
	defer flush(t, parent.Handle)
	if hex.EncodeToString(parent.Name.Buffer) != tok.SRKName {
		return nil, errors.New("the TPM is not the TPM of the enrollment (the SRK is different)")
	}
	load, err := tpm2.Load{
		ParentHandle: tpm2.AuthHandle{Handle: parent.Handle, Name: parent.Name, Auth: tpm2.PasswordAuth(nil)},
		InPrivate:    *priv,
		InPublic:     *pub,
	}.Execute(t)
	if err != nil {
		return nil, fmt.Errorf("load the sealed key: %w", err)
	}
	defer flush(t, load.ObjectHandle)
	opts := []tpm2.AuthOption{tpm2.Salted(parent.Handle, *parentPub), tpm2.AESEncryption(128, tpm2.EncryptOut)}
	if tok.PIN {
		opts = append(opts, tpm2.Auth(pinAuth(pin)))
	}
	sess, closeSess, err := tpm2.PolicySession(t, tpm2.TPMAlgSHA256, 16, opts...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeSess() }() // the session also ends with the TPM connection
	if _, err := (tpm2.PolicyPCR{PolicySession: sess.Handle(), Pcrs: pcrSelection(pcrSecBoot, pcrLock)}).Execute(t); err != nil {
		return nil, err
	}
	if tok.PIN {
		if _, err := (tpm2.PolicyAuthValue{PolicySession: sess.Handle()}).Execute(t); err != nil {
			return nil, err
		}
	}
	rsp, err := tpm2.Unseal{ItemHandle: tpm2.AuthHandle{Handle: load.ObjectHandle, Name: load.Name, Auth: sess}}.Execute(t)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "TPM_RC_LOCKOUT"):
			return nil, errLockout
		case tok.PIN && (strings.Contains(msg, "TPM_RC_AUTH_FAIL") || strings.Contains(msg, "TPM_RC_BAD_AUTH")):
			return nil, errWrongPIN
		}
		return nil, fmt.Errorf("the TPM policy refused the key (did the Secure Boot state change?): %w", err)
	}
	return rsp.OutData.Buffer, nil
}

// tpmUnlock is the /init step: unlock the LUKS2 device with the TPM. For a
// key with a PIN, askPIN gets the PIN (three tries). An empty PIN means:
// use the passphrase.
func tpmUnlock(dev string, askPIN func() (string, error), warn func(error)) error {
	id, tok, err := findTPMToken(dev)
	if err != nil {
		return err
	}
	var key []byte
	for try := 1; ; try++ {
		pin := ""
		if tok.PIN {
			if pin, err = askPIN(); err != nil {
				return err
			}
			if pin == "" {
				return errSkipped
			}
		}
		key, err = tpmUnseal(tok, pin)
		if err == nil {
			break
		}
		if !errors.Is(err, errWrongPIN) || try == 3 {
			return err
		}
		warn(err)
	}
	if len(tok.Keyslots) != 1 {
		return fmt.Errorf("token %s has no keyslot", id)
	}
	return runInput(key, "cryptsetup", "open", "--type", "luks2", "--key-file=-",
		"--key-slot", tok.Keyslots[0], dev, cryptName)
}

// cryptsetupFiles runs cryptsetup with secrets on extra file descriptors
// (/proc/self/fd/3, /proc/self/fd/4, ...), so that no secret is on the command line or
// in a file.
func cryptsetupFiles(secrets [][]byte, args ...string) error {
	cmd := exec.Command("cryptsetup", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	for _, s := range secrets {
		r, w, err := os.Pipe()
		if err != nil {
			return err
		}
		cmd.ExtraFiles = append(cmd.ExtraFiles, r)
		go func(w *os.File, s []byte) {
			// A short write gives cryptsetup a wrong key, and cryptsetup
			// then fails; the error of cryptsetup is the error to report.
			_, _ = w.Write(s)
			_ = w.Close()
		}(w, s)
	}
	err := cmd.Run()
	for _, f := range cmd.ExtraFiles {
		closeQuietly(f) // our copy of the read end
	}
	if err != nil {
		return fmt.Errorf("cryptsetup %s: %w", args[0], err)
	}
	return nil
}

func luksPartition() (*Partition, error) {
	return findPartition(luksLabel)
}

func readPassphrase(keyFile string) ([]byte, error) {
	if keyFile == "-" {
		b, err := io.ReadAll(os.Stdin)
		return bytes.TrimRight(b, "\n"), err
	}
	if keyFile != "" {
		b, err := os.ReadFile(keyFile)
		return bytes.TrimRight(b, "\n"), err
	}
	s, err := readSecret("Disk passphrase: ")
	return []byte(s), err
}

func freeKeyslot(m *luksMetadata) (string, error) {
	for i := 0; i < 32; i++ {
		if _, used := m.Keyslots[strconv.Itoa(i)]; !used {
			return strconv.Itoa(i), nil
		}
	}
	return "", errors.New("no free LUKS2 keyslot")
}

// removeTPMTokens removes all klos-tpm2 tokens and their keyslots, except
// the keyslot keep.
func removeTPMTokens(dev string, pass []byte, keep string) error {
	m, err := luksDump(dev)
	if err != nil {
		return err
	}
	var ids []string
	for id := range m.Tokens {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t := &tpmTokenData{}
		if json.Unmarshal(m.Tokens[id], t) != nil || t.Type != tpmToken {
			continue
		}
		if err := run("cryptsetup", "token", "remove", "--token-id", id, dev); err != nil {
			return err
		}
		for _, ks := range t.Keyslots {
			if ks == keep {
				continue
			}
			if err := cryptsetupFiles([][]byte{pass}, "luksKillSlot", "--batch-mode",
				"--key-file=/proc/self/fd/3", dev, ks); err != nil {
				return err
			}
		}
	}
	return nil
}

func cmdTPM(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: klos tpm status|enroll|remove")
	}
	switch args[0] {
	case "status":
		return tpmStatus()
	case "enroll":
		return tpmEnroll(args[1:])
	case "remove":
		return tpmRemove(args[1:])
	}
	return errors.New("usage: klos tpm status|enroll|remove")
}

func tpmEnroll(args []string) error {
	fs := flag.NewFlagSet("tpm enroll", flag.ContinueOnError)
	keyFile := fs.String("key-file", "", "read the current passphrase from this file (- is standard input)")
	usePIN := fs.Bool("pin", false, "the TPM also needs a PIN (the command asks for it)")
	pinFile := fs.String("pin-file", "", "read the PIN from this file (implies -pin)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireRoot(); err != nil {
		return err
	}
	if isLive() {
		return errors.New("enroll the TPM on the installed system")
	}
	pin := ""
	if *pinFile != "" {
		b, err := os.ReadFile(*pinFile)
		if err != nil {
			return err
		}
		pin = strings.TrimRight(string(b), "\n")
	} else if *usePIN {
		a, err := readSecret("New TPM PIN: ")
		if err != nil {
			return err
		}
		b, err := readSecret("Type the TPM PIN again: ")
		if err != nil {
			return err
		}
		if a != b {
			return errors.New("the PINs are not the same")
		}
		pin = a
	}
	hasPIN := *pinFile != "" || *usePIN
	if hasPIN && len(pin) < minPINLength {
		return fmt.Errorf("use a PIN with %d characters or more", minPINLength)
	}
	if secureBootState() != "enabled" {
		return errors.New("enroll needs Secure Boot: without it, PCR 7 does not protect the disk key (read docs/tpm.md)")
	}
	part, err := luksPartition()
	if err != nil {
		return err
	}
	t, err := openTPM(0)
	if err != nil {
		return err
	}
	defer closeQuietly(t)
	parent, parentPub, err := srk(t)
	if err != nil {
		return err
	}
	defer flush(t, parent.Handle)
	pcr7, err := readPCR(t, pcrSecBoot)
	if err != nil {
		return err
	}
	policy, err := pcrPolicy(pcr7, hasPIN)
	if err != nil {
		return err
	}
	raw := make([]byte, keyBytes)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	key := []byte(hex.EncodeToString(raw))
	sealed, err := tpm2.Create{
		ParentHandle: tpm2.AuthHandle{Handle: parent.Handle, Name: parent.Name, Auth: tpm2.PasswordAuth(nil)},
		InSensitive: tpm2.TPM2BSensitiveCreate{Sensitive: &tpm2.TPMSSensitiveCreate{
			UserAuth: tpm2.TPM2BAuth{Buffer: authFor(hasPIN, pin)},
			Data:     tpm2.NewTPMUSensitiveCreate(&tpm2.TPM2BSensitiveData{Buffer: key}),
		}},
		InPublic: tpm2.New2B(tpm2.TPMTPublic{
			Type:    tpm2.TPMAlgKeyedHash,
			NameAlg: tpm2.TPMAlgSHA256,
			ObjectAttributes: tpm2.TPMAObject{
				FixedTPM:    true,
				FixedParent: true,
				// With a PIN, the TPM lockout counts the wrong PINs.
				NoDA: !hasPIN,
			},
			AuthPolicy: tpm2.TPM2BDigest{Buffer: policy},
			Parameters: tpm2.NewTPMUPublicParms(tpm2.TPMAlgKeyedHash,
				&tpm2.TPMSKeyedHashParms{Scheme: tpm2.TPMTKeyedHashScheme{Scheme: tpm2.TPMAlgNull}}),
		}),
	}.Execute(t, tpm2.HMAC(tpm2.TPMAlgSHA256, 16,
		tpm2.Salted(parent.Handle, *parentPub), tpm2.AESEncryption(128, tpm2.EncryptIn)))
	if err != nil {
		return fmt.Errorf("seal the key in the TPM: %w", err)
	}
	pass, err := readPassphrase(*keyFile)
	if err != nil {
		return err
	}
	m, err := luksDump(part.Dev)
	if err != nil {
		return err
	}
	slot, err := freeKeyslot(m)
	if err != nil {
		return err
	}
	// The key has 256 bits of entropy, so a slow key derivation does not
	// add protection. PBKDF2 with the minimum count makes the unlock quick.
	if err := cryptsetupFiles([][]byte{pass, key}, "luksAddKey", "--batch-mode",
		"--key-file=/proc/self/fd/3", "--new-keyfile=/proc/self/fd/4", "--new-key-slot", slot,
		"--pbkdf", "pbkdf2", "--pbkdf-force-iterations", "1000", part.Dev); err != nil {
		return err
	}
	if err := removeTPMTokens(part.Dev, pass, slot); err != nil {
		return err
	}
	tok := tpmTokenData{
		Type:     tpmToken,
		Keyslots: []string{slot},
		Version:  1,
		SRKName:  hex.EncodeToString(parent.Name.Buffer),
		Public:   base64.StdEncoding.EncodeToString(tpm2.Marshal(sealed.OutPublic)),
		Private:  base64.StdEncoding.EncodeToString(tpm2.Marshal(sealed.OutPrivate)),
		PCRs:     []int{pcrSecBoot, pcrLock},
		PCR7:     hex.EncodeToString(pcr7),
		PIN:      hasPIN,
	}
	js, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	if err := cryptsetupFiles([][]byte{js}, "token", "import", "--json-file=/proc/self/fd/3", part.Dev); err != nil {
		return err
	}
	if hasPIN {
		fmt.Printf("The TPM key is in keyslot %s. From the next boot, the TPM and the PIN unlock the disk.\n", slot)
	} else {
		fmt.Printf("The TPM key is in keyslot %s. From the next boot, the TPM unlocks the disk.\n", slot)
	}
	fmt.Println("The passphrase stays valid. Keep it: you need it if the Secure Boot state changes.")
	return nil
}

func tpmRemove(args []string) error {
	fs := flag.NewFlagSet("tpm remove", flag.ContinueOnError)
	keyFile := fs.String("key-file", "", "read the passphrase from this file (- is standard input)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireRoot(); err != nil {
		return err
	}
	part, err := luksPartition()
	if err != nil {
		return err
	}
	if _, _, err := findTPMToken(part.Dev); err != nil {
		return err
	}
	pass, err := readPassphrase(*keyFile)
	if err != nil {
		return err
	}
	if err := removeTPMTokens(part.Dev, pass, ""); err != nil {
		return err
	}
	fmt.Println("The TPM key is removed. The disk needs the passphrase at each boot.")
	return nil
}

func tpmStatus() error {
	part, err := luksPartition()
	if err != nil {
		return err
	}
	_, tok, err := findTPMToken(part.Dev)
	if errors.Is(err, errNoToken) {
		fmt.Println("TPM key: not enrolled")
		return nil
	}
	if err != nil {
		return err
	}
	pinText := "no PIN"
	if tok.PIN {
		pinText = "PIN"
	}
	fmt.Printf("TPM key: enrolled (keyslot %s, PCRs %v, %s)\n", strings.Join(tok.Keyslots, ","), tok.PCRs, pinText)
	t, err := openTPM(0)
	if err != nil {
		fmt.Printf("TPM: %v\n", err)
		return nil
	}
	defer closeQuietly(t)
	if pcr7, err := readPCR(t, pcrSecBoot); err == nil {
		if hex.EncodeToString(pcr7) == tok.PCR7 {
			fmt.Println("PCR 7: the same as at the enrollment (the next boot can unlock)")
		} else {
			fmt.Println("PCR 7: different from the enrollment (the next boot needs the passphrase; run 'klos tpm enroll')")
		}
	}
	// Try to unseal the key now. After /init locked PCR 15, the TPM must
	// refuse. The key is not shown. A key with a PIN is not tried: a wrong
	// PIN counts toward the TPM lockout.
	if !tok.PIN {
		if _, err := tpmUnseal(tok, ""); err != nil {
			fmt.Println("unseal in this boot: refused (correct after the boot)")
		} else {
			fmt.Println("unseal in this boot: POSSIBLE (PCR 15 is not locked)")
		}
	}
	if pcr15, err := readPCR(t, pcrLock); err == nil {
		if bytes.Equal(pcr15, make([]byte, sha256.Size)) {
			fmt.Println("PCR 15: not locked")
		} else {
			fmt.Println("PCR 15: locked (the TPM does not unseal the key before the next boot)")
		}
	}
	return nil
}

func authFor(hasPIN bool, pin string) []byte {
	if hasPIN {
		return pinAuth(pin)
	}
	return nil
}
