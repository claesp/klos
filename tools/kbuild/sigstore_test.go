// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

// testRoot makes a trusted root with a CA and a log, and a leaf
// certificate of the CA for identity and issuer.
func testRoot(t *testing.T, identity, issuer string) (*TrustedRoot, *x509.Certificate, *ecdsa.PrivateKey, *ecdsa.PrivateKey) {
	t.Helper()
	now := time.Now()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	iss, _ := asn1.Marshal(issuer)
	leafTmpl := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Minute),
		NotAfter: now.Add(10 * time.Minute), EmailAddresses: []string{identity},
		ExtKeyUsage:     []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		ExtraExtensions: []pkix.Extension{{Id: oidIssuerV2, Value: iss}}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(leafDER)
	logKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	logDER, _ := x509.MarshalPKIXPublicKey(&logKey.PublicKey)
	logID := sha256.Sum256(logDER)
	tr := &TrustedRoot{Tlogs: make([]Tlog, 1), CertificateAuthorities: make([]CertAuthority, 1)}
	tr.Tlogs[0].PublicKey.RawBytes = base64.StdEncoding.EncodeToString(logDER)
	tr.Tlogs[0].LogID.KeyID = base64.StdEncoding.EncodeToString(logID[:])
	tr.CertificateAuthorities[0].CertChain.Certificates = []RawCert{{RawBytes: base64.StdEncoding.EncodeToString(caDER)}}
	return tr, leaf, leafKey, logKey
}

func TestCheckFulcioCert(t *testing.T) {
	tr, leaf, _, _ := testRoot(t, "rel@example.org", "https://issuer.example.org")
	now := time.Now()
	if err := tr.checkFulcioCert(leaf, now, "rel@example.org", "https://issuer.example.org"); err != nil {
		t.Fatalf("good certificate: %v", err)
	}
	if err := tr.checkFulcioCert(leaf, now, "other@example.org", "https://issuer.example.org"); err == nil {
		t.Error("wrong identity: no error")
	}
	if err := tr.checkFulcioCert(leaf, now, "rel@example.org", "https://other.example.org"); err == nil {
		t.Error("wrong issuer: no error")
	}
	if err := tr.checkFulcioCert(leaf, now.Add(time.Hour), "rel@example.org", "https://issuer.example.org"); err == nil {
		t.Error("time after the certificate: no error")
	}
	other, _, _, _ := testRoot(t, "rel@example.org", "https://issuer.example.org")
	if err := other.checkFulcioCert(leaf, now, "rel@example.org", "https://issuer.example.org"); err == nil {
		t.Error("other CA: no error")
	}
}

func TestVerifySET(t *testing.T) {
	tr, leaf, _, logKey := testRoot(t, "rel@example.org", "https://issuer.example.org")
	id, _ := base64.StdEncoding.DecodeString(tr.Tlogs[0].LogID.KeyID)
	e := RekorEntry{Body: "Ym9keQ==", IntegratedTime: time.Now().Unix(), LogIndex: 42, LogID: hex.EncodeToString(id)}
	payload := fmt.Sprintf(`{"body":%q,"integratedTime":%d,"logID":%q,"logIndex":%d}`, e.Body, e.IntegratedTime, e.LogID, e.LogIndex)
	h := sha256.Sum256([]byte(payload))
	e.SET, _ = ecdsa.SignASN1(rand.Reader, logKey, h[:])
	if err := tr.verifySET(e); err != nil {
		t.Fatalf("good SET: %v", err)
	}
	if _, err := checkTime(leaf, e); err != nil {
		t.Fatalf("time in the certificate validity: %v", err)
	}
	bad := e
	bad.LogIndex = 43
	if err := tr.verifySET(bad); err == nil {
		t.Error("changed entry: no error")
	}
	late := e
	late.IntegratedTime += 3600
	if _, err := checkTime(leaf, late); err == nil {
		t.Error("log time after the certificate: no error")
	}
}

func TestDSSEPAE(t *testing.T) {
	got := string(dssePAE("application/vnd.in-toto+json", []byte("{}")))
	if want := "DSSEv1 28 application/vnd.in-toto+json 2 {}"; got != want {
		t.Fatalf("dssePAE = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, "DSSEv1 ") {
		t.Fatal("no DSSEv1 prefix")
	}
}
