package certmanager

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func testShellyIssuer(t *testing.T) (*Issuer, *x509.Certificate) {
	t.Helper()
	now := time.Now().UTC()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "FBS Shelly Server CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewIssuer(ca, key)
	if err != nil {
		t.Fatal(err)
	}
	return issuer, ca
}

func TestShellyIssuedCertificatePreservesOriginalProfile(t *testing.T) {
	issuer, ca := testShellyIssuer(t)
	host := "2c41389b0d77.dynamic.utexas.edu"
	now := time.Now().UTC().Truncate(time.Second)
	bundle, err := issuer.Issue(host, now)
	if err != nil {
		t.Fatal(err)
	}
	leaf := bundle.Leaf
	if err := ValidateBundle(bundle, ca, host, now); err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != host || len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != host || len(leaf.IPAddresses) > 0 {
		t.Fatalf("unexpected certificate identity: CN=%s DNS=%v IP=%v", leaf.Subject.CommonName, leaf.DNSNames, leaf.IPAddresses)
	}
	if leaf.IsCA || !leaf.BasicConstraintsValid || leaf.KeyUsage != x509.KeyUsageDigitalSignature ||
		len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Fatal("leaf certificate does not match original OpenSSL server certificate extensions")
	}
	if len(leaf.SubjectKeyId) != sha1.Size || !bytes.Equal(leaf.AuthorityKeyId, ca.SubjectKeyId) {
		t.Fatal("leaf SKI/AKI missing or mismatched")
	}
	publicECDH, err := leaf.PublicKey.(*ecdsa.PublicKey).ECDH()
	if err != nil {
		t.Fatal(err)
	}
	expectedSKI := sha1.Sum(publicECDH.Bytes())
	if !bytes.Equal(leaf.SubjectKeyId, expectedSKI[:]) {
		t.Fatal("SKI differs from original OpenSSL subjectKeyIdentifier=hash profile")
	}
	if err := ValidateBundle(bundle, ca, "other.dynamic.utexas.edu", now); err == nil {
		t.Fatal("mismatched hostname was accepted")
	}
}

func TestShellyIssuerRejectsNonDDNSHostsWithoutSigning(t *testing.T) {
	issuer, _ := testShellyIssuer(t)
	now := time.Now().UTC()
	for _, host := range []string{
		"192.168.1.18", "2001:db8::1", "shelly.example.invalid",
		"dynamic.utexas.edu", "evil.dynamic.utexas.edu.evil.example",
		".dynamic.utexas.edu", "BAD.dynamic.utexas.edu",
		"shelly.dynamic.utexas.edu:443", "shelly.dynamic.utexas.edu/other",
	} {
		t.Run(host, func(t *testing.T) {
			if _, err := issuer.Issue(host, now); err == nil {
				t.Fatalf("unexpected issuance for %q", host)
			}
		})
	}
}
