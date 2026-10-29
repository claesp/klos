// SPDX-License-Identifier: BSD-2-Clause

package main

// Sigstore signatures: a short-lived certificate of the Fulcio CA binds a
// signing key to an identity (an e-mail address or a CI workflow), and the
// Rekor transparency log records the signature. The signed entry timestamp
// (SET) of Rekor proves that the signature was made while the certificate
// was valid. kbuild checks:
//   - the certificate chain to the Fulcio root of keys/sigstore-trusted-root.json,
//   - the identity and the OIDC issuer in the certificate (the recipe gives them),
//   - the signature of the file (or of the in-toto statement of the file),
//   - the SET of the Rekor entry with the Rekor key of the trusted root.
// Read docs/sources-and-keys.md.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const trustedRootFile = "sigstore-trusted-root.json"

// TrustedRoot is the part of a Sigstore trusted root that kbuild uses.
type TrustedRoot struct {
	Tlogs                  []Tlog          `json:"tlogs"`
	CertificateAuthorities []CertAuthority `json:"certificateAuthorities"`
}

// Tlog is a transparency log (Rekor) of a trusted root.
type Tlog struct {
	BaseURL   string `json:"baseUrl"`
	PublicKey struct {
		RawBytes   string `json:"rawBytes"`
		KeyDetails string `json:"keyDetails"`
	} `json:"publicKey"`
	LogID struct {
		KeyID string `json:"keyId"`
	} `json:"logId"`
}

// CertAuthority is a certificate authority (Fulcio) of a trusted root.
type CertAuthority struct {
	URI       string `json:"uri"`
	CertChain struct {
		Certificates []RawCert `json:"certificates"`
	} `json:"certChain"`
}

// RawCert is a DER certificate in base64.
type RawCert struct {
	RawBytes string `json:"rawBytes"`
}

func loadTrustedRoot() (*TrustedRoot, error) {
	b, err := os.ReadFile(filepath.Join(KEYS, trustedRootFile))
	if err != nil {
		return nil, err
	}
	tr := &TrustedRoot{}
	if err := json.Unmarshal(b, tr); err != nil {
		return nil, fmt.Errorf("%s: %w", trustedRootFile, err)
	}
	return tr, nil
}

// fulcioPools returns the root and intermediate certificates of the
// Fulcio CAs.
func (tr *TrustedRoot) fulcioPools() (roots, inter *x509.CertPool, err error) {
	roots, inter = x509.NewCertPool(), x509.NewCertPool()
	for _, ca := range tr.CertificateAuthorities {
		certs := ca.CertChain.Certificates
		for i, c := range certs {
			der, err := base64.StdEncoding.DecodeString(c.RawBytes)
			if err != nil {
				return nil, nil, err
			}
			cert, err := x509.ParseCertificate(der)
			if err != nil {
				return nil, nil, err
			}
			// The chain starts with the issuing CA and ends with the root.
			if i == len(certs)-1 {
				roots.AddCert(cert)
			} else {
				inter.AddCert(cert)
			}
		}
	}
	return roots, inter, nil
}

var (
	oidIssuerV1 = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 1}
	oidIssuerV2 = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 8}
)

// certIssuer returns the OIDC issuer in a Fulcio certificate.
func certIssuer(c *x509.Certificate) string {
	for _, e := range c.Extensions {
		switch {
		case e.Id.Equal(oidIssuerV2):
			var s string
			if _, err := asn1.Unmarshal(e.Value, &s); err == nil {
				return s
			}
		case e.Id.Equal(oidIssuerV1):
			return string(e.Value)
		}
	}
	return ""
}

// certIdentities returns the e-mail addresses and URIs in the subject
// alternative name of a certificate.
func certIdentities(c *x509.Certificate) []string {
	ids := append([]string{}, c.EmailAddresses...)
	for _, u := range c.URIs {
		ids = append(ids, u.String())
	}
	return ids
}

// checkFulcioCert checks the certificate chain at time t, the identity and
// the issuer.
func (tr *TrustedRoot) checkFulcioCert(c *x509.Certificate, t time.Time, identity, issuer string) error {
	roots, inter, err := tr.fulcioPools()
	if err != nil {
		return err
	}
	if _, err := c.Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter, CurrentTime: t,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}}); err != nil {
		return fmt.Errorf("certificate: %w", err)
	}
	if !contains(certIdentities(c), identity) {
		return fmt.Errorf("the certificate is for %v, not for %s", certIdentities(c), identity)
	}
	if got := certIssuer(c); got != issuer {
		return fmt.Errorf("the certificate has the OIDC issuer %q, not %q", got, issuer)
	}
	return nil
}

// verifyWithCert checks a signature of a SHA-256 digest with the key of a
// certificate (ECDSA or Ed25519).
func verifyWithCert(c *x509.Certificate, digest []byte, msg, sig []byte) error {
	switch k := c.PublicKey.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(k, digest, sig) {
			return errors.New("the signature is not valid")
		}
	case ed25519.PublicKey:
		if !ed25519.Verify(k, msg, sig) {
			return errors.New("the signature is not valid")
		}
	default:
		return fmt.Errorf("unknown key type %T", c.PublicKey)
	}
	return nil
}

// RekorEntry is a transparency log entry with its signed entry timestamp.
type RekorEntry struct {
	Body           string // base64
	IntegratedTime int64
	LogIndex       int64
	LogID          string // hex
	SET            []byte
}

// verifySET checks the signed entry timestamp of a Rekor entry with the
// key of the log in the trusted root.
func (tr *TrustedRoot) verifySET(e RekorEntry) error {
	for _, tl := range tr.Tlogs {
		id, err := base64.StdEncoding.DecodeString(tl.LogID.KeyID)
		if err != nil || hex.EncodeToString(id) != e.LogID {
			continue
		}
		der, err := base64.StdEncoding.DecodeString(tl.PublicKey.RawBytes)
		if err != nil {
			return err
		}
		pub, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			return err
		}
		// The payload is canonical JSON (sorted keys, no spaces).
		payload := fmt.Sprintf(`{"body":%q,"integratedTime":%d,"logID":%q,"logIndex":%d}`,
			e.Body, e.IntegratedTime, e.LogID, e.LogIndex)
		h := sha256.Sum256([]byte(payload))
		switch k := pub.(type) {
		case *ecdsa.PublicKey:
			if !ecdsa.VerifyASN1(k, h[:], e.SET) {
				return errors.New("the signed entry timestamp of Rekor is not valid")
			}
		default:
			return fmt.Errorf("Rekor key type %T is not supported for a signed entry timestamp", pub)
		}
		return nil
	}
	return fmt.Errorf("the log %s is not in the trusted root", e.LogID)
}

// checkTime checks that the log recorded the entry while the certificate
// was valid.
func checkTime(c *x509.Certificate, e RekorEntry) (time.Time, error) {
	t := time.Unix(e.IntegratedTime, 0)
	if t.Before(c.NotBefore) || t.After(c.NotAfter) {
		return t, fmt.Errorf("the log time %s is not in the validity of the certificate (%s to %s)",
			t.UTC(), c.NotBefore.UTC(), c.NotAfter.UTC())
	}
	return t, nil
}

func parseCertText(b []byte) (*x509.Certificate, error) {
	text := bytes.TrimSpace(b)
	if !bytes.HasPrefix(text, []byte("-----BEGIN")) {
		// cosign writes the PEM text in base64.
		d, err := base64.StdEncoding.DecodeString(string(text))
		if err != nil {
			return nil, fmt.Errorf("certificate: %w", err)
		}
		text = d
	}
	blk, _ := pem.Decode(text)
	if blk == nil {
		return nil, errors.New("certificate: no PEM data")
	}
	return x509.ParseCertificate(blk.Bytes)
}

// rekorFind finds the Rekor entry of a "hashedrekord" signature of a file
// (cosign sign-blob) with the signature sig and the certificate cert.
func rekorFind(ctx context.Context, baseURL, sha string, sig []byte, cert *x509.Certificate) (RekorEntry, error) {
	var uuids []string
	if err := osvPost(ctx, baseURL+"/api/v1/index/retrieve", map[string]string{"hash": "sha256:" + sha}, &uuids); err != nil {
		return RekorEntry{}, err
	}
	for _, u := range uuids {
		b, err := httpGet(ctx, baseURL+"/api/v1/log/entries/"+u)
		if err != nil {
			return RekorEntry{}, err
		}
		var res map[string]struct {
			Body           string `json:"body"`
			IntegratedTime int64  `json:"integratedTime"`
			LogID          string `json:"logID"`
			LogIndex       int64  `json:"logIndex"`
			Verification   struct {
				SET string `json:"signedEntryTimestamp"`
			} `json:"verification"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return RekorEntry{}, err
		}
		for _, e := range res {
			body, err := base64.StdEncoding.DecodeString(e.Body)
			if err != nil {
				continue
			}
			var hr struct {
				Kind string `json:"kind"`
				Spec struct {
					Signature struct {
						Content   string `json:"content"`
						PublicKey struct {
							Content string `json:"content"`
						} `json:"publicKey"`
					} `json:"signature"`
					Data struct {
						Hash struct {
							Value string `json:"value"`
						} `json:"hash"`
					} `json:"data"`
				} `json:"spec"`
			}
			if json.Unmarshal(body, &hr) != nil || hr.Kind != "hashedrekord" || hr.Spec.Data.Hash.Value != sha {
				continue
			}
			esig, err1 := base64.StdEncoding.DecodeString(hr.Spec.Signature.Content)
			ecert, err2 := base64.StdEncoding.DecodeString(hr.Spec.Signature.PublicKey.Content)
			if err1 != nil || err2 != nil || !bytes.Equal(esig, sig) {
				continue
			}
			if c, err := parseCertText(ecert); err != nil || !c.Equal(cert) {
				continue
			}
			set, err := base64.StdEncoding.DecodeString(e.Verification.SET)
			if err != nil {
				return RekorEntry{}, err
			}
			return RekorEntry{Body: e.Body, IntegratedTime: e.IntegratedTime, LogIndex: e.LogIndex,
				LogID: e.LogID, SET: set}, nil
		}
	}
	return RekorEntry{}, errors.New("no Rekor entry has this signature and certificate")
}

// verifySigstoreBlob checks a cosign "sign-blob" signature (sigPath, in
// base64) and certificate (certPath) of a file.
func verifySigstoreBlob(s Source, sigPath, certPath, dataPath string) error {
	tr, err := loadTrustedRoot()
	if err != nil {
		return err
	}
	sigText, err := os.ReadFile(sigPath)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigText)))
	if err != nil {
		return fmt.Errorf("signature: %w", err)
	}
	certText, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}
	cert, err := parseCertText(certText)
	if err != nil {
		return err
	}
	sha := sha256File(dataPath)
	digest, _ := hex.DecodeString(sha)
	if err := verifyWithCert(cert, digest, nil, sig); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), upstreamTimeout)
	defer cancel()
	var lastErr error
	for _, tl := range tr.Tlogs {
		e, err := rekorFind(ctx, tl.BaseURL, sha, sig, cert)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", tl.BaseURL, err)
			continue
		}
		if err := tr.verifySET(e); err != nil {
			return err
		}
		t, err := checkTime(cert, e)
		if err != nil {
			return err
		}
		if err := tr.checkFulcioCert(cert, t, s.Identity, s.Issuer); err != nil {
			return err
		}
		logf("Sigstore signature by %s (%s), Rekor entry %d at %s", s.Identity, s.Issuer, e.LogIndex, t.UTC().Format(time.RFC3339))
		return nil
	}
	return lastErr
}

// Provenance is the PyPI provenance object of a file (PEP 740).
type Provenance struct {
	AttestationBundles []struct {
		Attestations []struct {
			Envelope struct {
				Statement string `json:"statement"`
				Signature string `json:"signature"`
			} `json:"envelope"`
			VerificationMaterial struct {
				Certificate         string `json:"certificate"`
				TransparencyEntries []struct {
					CanonicalizedBody string `json:"canonicalizedBody"`
					IntegratedTime    string `json:"integratedTime"`
					LogIndex          string `json:"logIndex"`
					LogID             struct {
						KeyID string `json:"keyId"`
					} `json:"logId"`
					InclusionPromise struct {
						SET string `json:"signedEntryTimestamp"`
					} `json:"inclusionPromise"`
				} `json:"transparency_entries"`
			} `json:"verification_material"`
		} `json:"attestations"`
	} `json:"attestation_bundles"`
}

// dssePAE is the pre-authentication encoding of DSSE.
func dssePAE(payloadType string, payload []byte) []byte {
	return []byte(fmt.Sprintf("DSSEv1 %d %s %d %s", len(payloadType), payloadType, len(payload), payload))
}

// verifyPyPIProvenance checks a PyPI attestation of a file: the in-toto
// statement names the file and its SHA-256 value, and the certificate is
// for the publish workflow of the project.
func verifyPyPIProvenance(s Source, provPath, dataPath string) error {
	tr, err := loadTrustedRoot()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(provPath)
	if err != nil {
		return err
	}
	var prov Provenance
	if err := json.Unmarshal(b, &prov); err != nil {
		return fmt.Errorf("provenance: %w", err)
	}
	sha := sha256File(dataPath)
	var lastErr error = errors.New("the provenance has no attestations")
	for _, bundle := range prov.AttestationBundles {
		for _, a := range bundle.Attestations {
			if err := checkAttestation(tr, s, sha, a.Envelope.Statement, a.Envelope.Signature,
				a.VerificationMaterial.Certificate, func() ([]RekorEntry, error) {
					var es []RekorEntry
					for _, te := range a.VerificationMaterial.TransparencyEntries {
						id, err := base64.StdEncoding.DecodeString(te.LogID.KeyID)
						if err != nil {
							return nil, err
						}
						set, err := base64.StdEncoding.DecodeString(te.InclusionPromise.SET)
						if err != nil {
							return nil, err
						}
						it, err1 := strconv.ParseInt(te.IntegratedTime, 10, 64)
						li, err2 := strconv.ParseInt(te.LogIndex, 10, 64)
						if err1 != nil || err2 != nil {
							return nil, errors.New("bad transparency entry")
						}
						es = append(es, RekorEntry{Body: te.CanonicalizedBody, IntegratedTime: it, LogIndex: li,
							LogID: hex.EncodeToString(id), SET: set})
					}
					return es, nil
				}); err != nil {
				lastErr = err
				continue
			}
			return nil
		}
	}
	return lastErr
}

func checkAttestation(tr *TrustedRoot, s Source, sha, statementB64, sigB64, certB64 string,
	entries func() ([]RekorEntry, error)) error {
	statement, err := base64.StdEncoding.DecodeString(statementB64)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return err
	}
	der, err := base64.StdEncoding.DecodeString(certB64)
	if err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return err
	}
	pae := dssePAE("application/vnd.in-toto+json", statement)
	h := sha256.Sum256(pae)
	if err := verifyWithCert(cert, h[:], pae, sig); err != nil {
		return fmt.Errorf("attestation: %w", err)
	}
	var st struct {
		Subject []struct {
			Name   string            `json:"name"`
			Digest map[string]string `json:"digest"`
		} `json:"subject"`
	}
	if err := json.Unmarshal(statement, &st); err != nil {
		return err
	}
	found := false
	for _, sub := range st.Subject {
		found = found || (sub.Name == s.filename() && sub.Digest["sha256"] == sha)
	}
	if !found {
		return fmt.Errorf("the attestation is not for %s with SHA-256 %s", s.filename(), sha)
	}
	es, err := entries()
	if err != nil {
		return err
	}
	ph := sha256.Sum256(statement)
	for _, e := range es {
		// The entry must be the one of this signature and statement.
		body, err := base64.StdEncoding.DecodeString(e.Body)
		if err != nil {
			continue
		}
		var dsse struct {
			Kind string `json:"kind"`
			Spec struct {
				PayloadHash struct {
					Value string `json:"value"`
				} `json:"payloadHash"`
				Signatures []struct {
					Signature string `json:"signature"`
					Verifier  string `json:"verifier"`
				} `json:"signatures"`
			} `json:"spec"`
		}
		if json.Unmarshal(body, &dsse) != nil || dsse.Kind != "dsse" || dsse.Spec.PayloadHash.Value != hex.EncodeToString(ph[:]) {
			continue
		}
		match := false
		for _, ds := range dsse.Spec.Signatures {
			v, err := base64.StdEncoding.DecodeString(ds.Verifier)
			if err != nil {
				continue
			}
			c, err := parseCertText(v)
			match = match || (err == nil && c.Equal(cert) && ds.Signature == sigB64)
		}
		if !match {
			continue
		}
		if err := tr.verifySET(e); err != nil {
			return err
		}
		t, err := checkTime(cert, e)
		if err != nil {
			return err
		}
		if err := tr.checkFulcioCert(cert, t, s.Identity, s.Issuer); err != nil {
			return err
		}
		logf("PyPI attestation by %s, Rekor entry %d at %s", s.Identity, e.LogIndex, t.UTC().Format(time.RFC3339))
		return nil
	}
	return errors.New("the attestation has no Rekor entry with a signed entry timestamp")
}
