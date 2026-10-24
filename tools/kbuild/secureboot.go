// SPDX-License-Identifier: BSD-2-Clause

package main

// UEFI Secure Boot. Read docs/secure-boot.md.
//
// kbuild makes three key pairs one time (secrets/secureboot/):
//
//	PK   platform key: signs changes to KEK (and to PK)
//	KEK  key exchange key: signs changes to db
//	db   signature database key: signs the EFI binaries
//
// It signs klos.efi and klos-live.efi with the db key (Authenticode,
// PKCS #7 with SHA-256), and it makes the signed variable updates
// PK.auth, KEK.auth and db.auth that "klos secureboot enroll" writes to the
// firmware. RSA PKCS #1 v1.5 signatures are deterministic, and the signed
// data has no time of signing, so the signed files are reproducible.

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	guidCertX509     = "a5c059a1-94e4-4aa7-87b5-ab155c2bf072"
	guidCertPKCS7    = "4aafd29d-68df-49ee-8aa9-347d375665a7"
	guidEFIGlobal    = "8be4df61-93ca-11d2-aa0d-00e098032b8c"
	guidImageSecDB   = "d719b2cb-3d3a-4596-a3bc-dad00e67656f"
	sbVarAttributes  = 0x27 // NV, BS, RT, time-based authenticated write
	winCertRevision  = 0x0200
	winCertPKCS      = 0x0002
	winCertEFIGUID   = 0x0ef1
	sbKeyBits        = 2048 // the size that all UEFI firmware supports
	sbCertValidYears = 30
)

var sbKeyNames = []string{"PK", "KEK", "db"}

type sbKey struct {
	name string
	key  *rsa.PrivateKey
	cert *x509.Certificate
}

func sbDir() string { return filepath.Join(SECRETS, "secureboot") }

// guidBytes returns a GUID in the EFI byte order (the first three fields
// are little-endian).
func guidBytes(s string) []byte {
	b, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(b) != 16 {
		die("bad GUID %q", s)
	}
	out := make([]byte, 16)
	out[0], out[1], out[2], out[3] = b[3], b[2], b[1], b[0]
	out[4], out[5] = b[5], b[4]
	out[6], out[7] = b[7], b[6]
	copy(out[8:], b[8:])
	return out
}

// ensureSecureBootKeys makes the PK, KEK and db keys and the owner GUID if
// they do not exist, and returns the keys.
func ensureSecureBootKeys() map[string]*sbKey {
	dir := sbDir()
	mkdirAll(dir)
	check(os.Chmod(dir, 0o700))
	if !exists(filepath.Join(dir, "owner.guid")) {
		g := make([]byte, 16)
		_, err := rand.Read(g)
		check(err)
		g[6] = g[6]&0x0f | 0x40
		g[8] = g[8]&0x3f | 0x80
		writeFile(filepath.Join(dir, "owner.guid"),
			fmt.Sprintf("%x-%x-%x-%x-%x\n", g[0:4], g[4:6], g[6:8], g[8:10], g[10:16]), 0o644)
	}
	keys := map[string]*sbKey{}
	for _, n := range sbKeyNames {
		keyPath, crtPath := filepath.Join(dir, n+".key"), filepath.Join(dir, n+".crt")
		if !exists(keyPath) {
			makeSecureBootKey(n, keyPath, crtPath)
		}
		keys[n] = loadSecureBootKey(n, keyPath, crtPath)
	}
	return keys
}

func makeSecureBootKey(name, keyPath, crtPath string) {
	key, err := rsa.GenerateKey(rand.Reader, sbKeyBits)
	check(err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	check(err)
	now := time.Now().UTC().Truncate(24 * time.Hour)
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{"KLOS"}, CommonName: "KLOS Secure Boot " + name},
		NotBefore:             now.Add(-24 * time.Hour),
		NotAfter:              now.AddDate(sbCertValidYears, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  name != "db",
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	check(err)
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	check(err)
	writeFile(keyPath, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb})), 0o600)
	writeFile(crtPath, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), 0o644)
	logf("made the Secure Boot key %s (%s)", name, rel(keyPath))
}

func loadSecureBootKey(name, keyPath, crtPath string) *sbKey {
	kb, err := os.ReadFile(keyPath)
	check(err)
	blk, _ := pem.Decode(kb)
	if blk == nil {
		die("%s: no PEM data", rel(keyPath))
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	check(err)
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		die("%s: not an RSA key", rel(keyPath))
	}
	cb, err := os.ReadFile(crtPath)
	check(err)
	cblk, _ := pem.Decode(cb)
	if cblk == nil {
		die("%s: no PEM data", rel(crtPath))
	}
	cert, err := x509.ParseCertificate(cblk.Bytes)
	check(err)
	return &sbKey{name: name, key: rk, cert: cert}
}

// --- DER encoding ---

func derLen(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte(n)}, b...)
		n >>= 8
	}
	return append([]byte{0x80 | byte(len(b))}, b...)
}

func der(tag byte, parts ...[]byte) []byte {
	var c []byte
	for _, p := range parts {
		c = append(c, p...)
	}
	out := append([]byte{tag}, derLen(len(c))...)
	return append(out, c...)
}

func derOID(ids ...int) []byte {
	b, err := asn1.Marshal(asn1.ObjectIdentifier(ids))
	check(err)
	return b
}

func derInt(i *big.Int) []byte {
	b, err := asn1.Marshal(i)
	check(err)
	return b
}

var (
	derNull         = []byte{0x05, 0x00}
	oidSHA256       = derOID(2, 16, 840, 1, 101, 3, 4, 2, 1)
	oidRSA          = derOID(1, 2, 840, 113549, 1, 1, 1)
	oidData         = derOID(1, 2, 840, 113549, 1, 7, 1)
	oidSignedData   = derOID(1, 2, 840, 113549, 1, 7, 2)
	oidContentType  = derOID(1, 2, 840, 113549, 1, 9, 3)
	oidMsgDigest    = derOID(1, 2, 840, 113549, 1, 9, 4)
	oidSpcIndirect  = derOID(1, 3, 6, 1, 4, 1, 311, 2, 1, 4)
	oidSpcOpusInfo  = derOID(1, 3, 6, 1, 4, 1, 311, 2, 1, 12)
	oidSpcPEImage   = derOID(1, 3, 6, 1, 4, 1, 311, 2, 1, 15)
	derAlgSHA256    = der(0x30, oidSHA256, derNull)
	derAlgRSA       = der(0x30, oidRSA, derNull)
	derIntOne       = []byte{0x02, 0x01, 0x01}
	obsoleteBMP     = bmpString("<<<Obsolete>>>")
	errNotSigned    = errors.New("the PE file has no signature")
	errBadSignature = errors.New("the signature is not valid")
)

func bmpString(s string) []byte {
	var b []byte
	for _, r := range utf16.Encode([]rune(s)) {
		b = append(b, byte(r>>8), byte(r))
	}
	return b
}

// signedData makes a PKCS #7 SignedData. With attrs, the signature is over
// the DER SET of the attributes; without attrs, it is over toSign.
func signedData(k *sbKey, contentInfo []byte, attrs [][]byte, toSign []byte) []byte {
	var signerAttrs []byte
	signed := toSign
	if attrs != nil {
		// DER: a SET OF has its elements in sorted order.
		sort.Slice(attrs, func(i, j int) bool { return bytes.Compare(attrs[i], attrs[j]) < 0 })
		set := der(0x31, attrs...)
		signed = set
		signerAttrs = append([]byte{0xa0}, set[1:]...) // [0] IMPLICIT
	}
	digest := sha256.Sum256(signed)
	sig, err := rsa.SignPKCS1v15(nil, k.key, crypto.SHA256, digest[:])
	check(err)
	signer := der(0x30, derIntOne,
		der(0x30, k.cert.RawIssuer, derInt(k.cert.SerialNumber)),
		derAlgSHA256, signerAttrs, derAlgRSA, der(0x04, sig))
	return der(0x30, derIntOne, der(0x31, derAlgSHA256), contentInfo,
		der(0xa0, k.cert.Raw), der(0x31, signer))
}

// --- PE / Authenticode ---

type peFile struct {
	checksumOff   int
	certDirOff    int
	sizeOfHeaders int
	sections      [][2]int // file offset and size of the raw data
	certOff       int
	certSize      int
}

func parsePE(b []byte) (*peFile, error) {
	if len(b) < 0x40 || b[0] != 'M' || b[1] != 'Z' {
		return nil, errors.New("not a PE file")
	}
	pe := int(binary.LittleEndian.Uint32(b[0x3c:]))
	if pe+24 > len(b) || string(b[pe:pe+4]) != "PE\x00\x00" {
		return nil, errors.New("no PE header")
	}
	coff := pe + 4
	nsec := int(binary.LittleEndian.Uint16(b[coff+2:]))
	optSize := int(binary.LittleEndian.Uint16(b[coff+16:]))
	opt := coff + 20
	if opt+optSize > len(b) || binary.LittleEndian.Uint16(b[opt:]) != 0x20b {
		return nil, errors.New("not a PE32+ file")
	}
	if binary.LittleEndian.Uint32(b[opt+108:]) < 5 {
		return nil, errors.New("the PE file has no certificate table entry")
	}
	p := &peFile{
		checksumOff:   opt + 64,
		certDirOff:    opt + 112 + 4*8,
		sizeOfHeaders: int(binary.LittleEndian.Uint32(b[opt+60:])),
	}
	p.certOff = int(binary.LittleEndian.Uint32(b[p.certDirOff:]))
	p.certSize = int(binary.LittleEndian.Uint32(b[p.certDirOff+4:]))
	sec := opt + optSize
	for i := 0; i < nsec; i++ {
		s := b[sec+40*i:]
		size := int(binary.LittleEndian.Uint32(s[16:]))
		ptr := int(binary.LittleEndian.Uint32(s[20:]))
		if size > 0 {
			if ptr+size > len(b) {
				return nil, errors.New("a PE section is outside the file")
			}
			p.sections = append(p.sections, [2]int{ptr, size})
		}
	}
	sort.Slice(p.sections, func(i, j int) bool { return p.sections[i][0] < p.sections[j][0] })
	return p, nil
}

// authenticodeDigest is the PE image hash of the Authenticode specification.
func authenticodeDigest(b []byte, p *peFile) []byte {
	h := sha256.New()
	h.Write(b[:p.checksumOff])
	h.Write(b[p.checksumOff+4 : p.certDirOff])
	h.Write(b[p.certDirOff+8 : p.sizeOfHeaders])
	sum := p.sizeOfHeaders
	for _, s := range p.sections {
		h.Write(b[s[0] : s[0]+s[1]])
		sum += s[1]
	}
	end := len(b) - p.certSize
	if sum < end {
		h.Write(b[sum:end])
	}
	return h.Sum(nil)
}

func peChecksum(b []byte, off int) uint32 {
	var sum uint64
	for i := 0; i+1 < len(b); i += 2 {
		if i == off || i == off+2 {
			continue
		}
		sum += uint64(binary.LittleEndian.Uint16(b[i:]))
		sum = (sum & 0xffff) + (sum >> 16)
	}
	if len(b)%2 == 1 {
		sum += uint64(b[len(b)-1])
		sum = (sum & 0xffff) + (sum >> 16)
	}
	sum = (sum & 0xffff) + (sum >> 16)
	return uint32(sum) + uint32(len(b))
}

// spcIndirectData is the Authenticode content: the PE image hash.
func spcIndirectData(digest []byte) []byte {
	peImage := der(0x30, []byte{0x03, 0x01, 0x00}, // flags: empty BIT STRING
		der(0xa0, der(0xa2, der(0x80, obsoleteBMP))))
	return der(0x30, der(0x30, oidSpcPEImage, peImage),
		der(0x30, derAlgSHA256, der(0x04, digest)))
}

// derContents returns the contents of a DER element (without tag and length).
func derContents(b []byte) []byte {
	var rv asn1.RawValue
	if _, err := asn1.Unmarshal(b, &rv); err != nil {
		die("internal DER error: %v", err)
	}
	return rv.Bytes
}

// signPE adds an Authenticode signature to a PE file (an EFI binary).
func signPE(path string, k *sbKey) {
	b, err := os.ReadFile(path)
	check(err)
	p, err := parsePE(b)
	if err != nil {
		die("%s: %v", rel(path), err)
	}
	if p.certSize != 0 {
		die("%s: the file has a signature already", rel(path))
	}
	for len(b)%8 != 0 {
		b = append(b, 0)
	}
	spc := spcIndirectData(authenticodeDigest(b, p))
	// Authenticode: the message digest is over the contents of the
	// SpcIndirectDataContent, without its tag and length.
	md := sha256.Sum256(derContents(spc))
	attrs := [][]byte{
		der(0x30, oidContentType, der(0x31, oidSpcIndirect)),
		der(0x30, oidSpcOpusInfo, der(0x31, der(0x30))),
		der(0x30, oidMsgDigest, der(0x31, der(0x04, md[:]))),
	}
	sd := signedData(k, der(0x30, oidSpcIndirect, der(0xa0, spc)), attrs, nil)
	p7 := der(0x30, oidSignedData, der(0xa0, sd))
	cert := make([]byte, 8, 8+len(p7)+8)
	binary.LittleEndian.PutUint32(cert[0:], uint32(8+len(p7)))
	binary.LittleEndian.PutUint16(cert[4:], winCertRevision)
	binary.LittleEndian.PutUint16(cert[6:], winCertPKCS)
	cert = append(cert, p7...)
	for len(cert)%8 != 0 {
		cert = append(cert, 0)
	}
	off := len(b)
	b = append(b, cert...)
	binary.LittleEndian.PutUint32(b[p.certDirOff:], uint32(off))
	binary.LittleEndian.PutUint32(b[p.certDirOff+4:], uint32(len(cert)))
	binary.LittleEndian.PutUint32(b[p.checksumOff:], peChecksum(b, p.checksumOff))
	writeFile(path, string(b), 0o644)
	if err := verifyPE(path, k.cert); err != nil {
		die("%s: the new signature does not verify: %v", rel(path), err)
	}
}

func derChildren(b []byte) ([]asn1.RawValue, error) {
	var out []asn1.RawValue
	for len(b) > 0 {
		var rv asn1.RawValue
		rest, err := asn1.Unmarshal(b, &rv)
		if err != nil {
			return nil, err
		}
		out = append(out, rv)
		b = rest
	}
	return out, nil
}

// verifyPE checks the Authenticode signature of a PE file with a
// certificate: the image hash, the message digest and the RSA signature.
func verifyPE(path string, cert *x509.Certificate) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p, err := parsePE(b)
	if err != nil {
		return err
	}
	if p.certSize == 0 || p.certOff+p.certSize != len(b) || p.certSize < 8 {
		return errNotSigned
	}
	wc := b[p.certOff:]
	n := int(binary.LittleEndian.Uint32(wc))
	if n < 8 || n > len(wc) || binary.LittleEndian.Uint16(wc[6:]) != winCertPKCS {
		return errBadSignature
	}
	// ContentInfo -> [0] -> SignedData
	ci, err := derChildren(derContents(wc[8:n]))
	if err != nil || len(ci) != 2 {
		return errBadSignature
	}
	sdParts, err := derChildren(derContents(ci[1].Bytes))
	if err != nil || len(sdParts) < 4 {
		return errBadSignature
	}
	// encapsulated content: SEQUENCE { OID, [0] { SpcIndirectDataContent } }
	enc, err := derChildren(sdParts[2].Bytes)
	if err != nil || len(enc) != 2 {
		return errBadSignature
	}
	spc := enc[1].Bytes
	spcParts, err := derChildren(derContents(spc))
	if err != nil || len(spcParts) != 2 {
		return errBadSignature
	}
	di, err := derChildren(spcParts[1].Bytes)
	if err != nil || len(di) != 2 {
		return errBadSignature
	}
	if !bytes.Equal(di[1].Bytes, authenticodeDigest(b[:p.certOff], &peFile{
		checksumOff: p.checksumOff, certDirOff: p.certDirOff,
		sizeOfHeaders: p.sizeOfHeaders, sections: p.sections})) {
		return errors.New("the image hash does not agree with the signature")
	}
	signers, err := derChildren(sdParts[len(sdParts)-1].Bytes)
	if err != nil || len(signers) != 1 {
		return errBadSignature
	}
	si, err := derChildren(signers[0].Bytes)
	if err != nil || len(si) != 6 {
		return errBadSignature
	}
	attrSet := append([]byte{0x31}, si[3].FullBytes[1:]...)
	attrs, err := derChildren(si[3].Bytes)
	if err != nil {
		return errBadSignature
	}
	md := sha256.Sum256(derContents(spc))
	found := false
	for _, a := range attrs {
		f, err := derChildren(a.Bytes)
		if err == nil && len(f) == 2 && bytes.Equal(f[0].FullBytes, oidMsgDigest) {
			v, err := derChildren(f[1].Bytes)
			found = err == nil && len(v) == 1 && bytes.Equal(v[0].Bytes, md[:])
		}
	}
	if !found {
		return errors.New("the message digest does not agree")
	}
	h := sha256.Sum256(attrSet)
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("not an RSA certificate")
	}
	if rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], si[5].Bytes) != nil {
		return errBadSignature
	}
	return nil
}

// --- Authenticated variables (PK.auth, KEK.auth, db.auth) ---

// efiSignatureList makes an EFI_SIGNATURE_LIST with one X.509 certificate.
func efiSignatureList(owner string, cert []byte) []byte {
	sigSize := 16 + len(cert)
	b := append([]byte{}, guidBytes(guidCertX509)...)
	var hdr [12]byte
	binary.LittleEndian.PutUint32(hdr[0:], uint32(16+12+sigSize))
	binary.LittleEndian.PutUint32(hdr[4:], 0)
	binary.LittleEndian.PutUint32(hdr[8:], uint32(sigSize))
	b = append(b, hdr[:]...)
	b = append(b, guidBytes(owner)...)
	return append(b, cert...)
}

func efiTime(t time.Time) []byte {
	t = t.UTC()
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b[0:], uint16(t.Year()))
	b[2], b[3] = byte(t.Month()), byte(t.Day())
	b[4], b[5], b[6] = byte(t.Hour()), byte(t.Minute()), byte(t.Second())
	return b
}

// authVariable makes an EFI_VARIABLE_AUTHENTICATION_2 update: the time
// stamp, a PKCS #7 signature by signer, and the data.
func authVariable(name, guid string, data []byte, signer *sbKey, ts time.Time) []byte {
	tsb := efiTime(ts)
	var signed []byte
	for _, r := range utf16.Encode([]rune(name)) {
		signed = append(signed, byte(r), byte(r>>8))
	}
	signed = append(signed, guidBytes(guid)...)
	var attrs [4]byte
	binary.LittleEndian.PutUint32(attrs[:], sbVarAttributes)
	signed = append(signed, attrs[:]...)
	signed = append(signed, tsb...)
	signed = append(signed, data...)
	// The firmware takes the SignedData without the ContentInfo wrapper.
	p7 := signedData(signer, der(0x30, oidData), nil, signed)
	win := make([]byte, 8)
	binary.LittleEndian.PutUint32(win[0:], uint32(8+16+len(p7)))
	binary.LittleEndian.PutUint16(win[4:], winCertRevision)
	binary.LittleEndian.PutUint16(win[6:], winCertEFIGUID)
	win = append(win, guidBytes(guidCertPKCS7)...)
	win = append(win, p7...)
	out := append(tsb, win...)
	return append(out, data...)
}

// writeSecureBootFiles writes the enrollment files into dir: PK.auth,
// KEK.auth and db.auth (signed variable updates) and the certificates.
func writeSecureBootFiles(keys map[string]*sbKey, dir string) {
	mkdirAll(dir)
	b, err := os.ReadFile(filepath.Join(sbDir(), "owner.guid"))
	check(err)
	owner := strings.TrimSpace(string(b))
	ts := time.Unix(SDE, 0)
	vars := []struct {
		name, guid string
		key        *sbKey // the certificate in the variable
		signer     *sbKey
	}{
		{"PK", guidEFIGlobal, keys["PK"], keys["PK"]},
		{"KEK", guidEFIGlobal, keys["KEK"], keys["PK"]},
		{"db", guidImageSecDB, keys["db"], keys["KEK"]},
	}
	for _, v := range vars {
		esl := efiSignatureList(owner, v.key.cert.Raw)
		writeFile(filepath.Join(dir, v.name+".auth"), string(authVariable(v.name, v.guid, esl, v.signer, ts)), 0o644)
		writeFile(filepath.Join(dir, v.name+".crt"),
			string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: v.key.cert.Raw})), 0o644)
	}
}
