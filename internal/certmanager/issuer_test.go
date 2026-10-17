package certmanager

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"testing"
	"time"
)

type countingSigner struct {
	crypto.Signer
	signs int
}

func (s *countingSigner) Sign(rnd io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	s.signs++
	return s.Signer.Sign(rnd, digest, opts)
}

func newTestCA(t *testing.T, now time.Time, expiry time.Time) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Simulated PIV Issuing CA"},
		NotBefore:    now.Add(-time.Hour), NotAfter: expiry,
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:   x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		MaxPathLen: 0, MaxPathLenZero: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func TestIssueValidBundleUsingSimulatedHardwareSigner(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	ca, key := newTestCA(t, now, now.Add(5*365*24*time.Hour))
	signer := &countingSigner{Signer: key}
	issuer, err := NewIssuer(ca, signer)
	if err != nil {
		t.Fatal(err)
	}
	observed := Observation{
		Host:      "a085e3b5325c.dynamic.utexas.edu",
		NotBefore: now.Add(-700 * 24 * time.Hour),
		NotAfter:  now.Add(20 * 24 * time.Hour),
	}
	bundle, decision, err := PrepareIfDue(now, observed, DefaultRenewBefore, issuer)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Due || bundle == nil || signer.signs != 1 {
		t.Fatalf("unexpected issuance result: decision=%+v bundle=%v signs=%d", decision, bundle != nil, signer.signs)
	}
	if err := ValidateBundle(bundle, ca, observed.Host, now); err != nil {
		t.Fatal(err)
	}
	if bundle.Leaf.NotAfter.Sub(now) > defaultLeafLifetime || bundle.Leaf.NotAfter.Sub(now) < 824*24*time.Hour {
		t.Fatalf("unexpected lifetime: %s", bundle.Leaf.NotAfter.Sub(now))
	}
	if _, decision, err := PrepareIfDue(now, Observation{Host: observed.Host, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(300 * 24 * time.Hour)}, DefaultRenewBefore, issuer); err != nil || decision.Due || signer.signs != 1 {
		t.Fatalf("healthy certificate triggered signing: decision=%+v err=%v count=%d", decision, err, signer.signs)
	}
}

func TestRejectInvalidSignerAndBadBundles(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	ca, key := newTestCA(t, now, now.Add(4*365*24*time.Hour))
	_, otherKey := newTestCA(t, now, now.Add(4*365*24*time.Hour))
	if _, err := NewIssuer(ca, otherKey); err == nil {
		t.Fatal("signer does not match issuer CA")
	}
	issuer, err := NewIssuer(ca, key)
	if err != nil {
		t.Fatal(err)
	}
	for _, badHost := range []string{"", "foo", "foo.example.test:443", "*.example.test", "UPPER.example.test", "foo/example.test", "-foo.example.test"} {
		if _, err := issuer.Issue(badHost, now); err == nil {
			t.Errorf("accepted invalid host %q", badHost)
		}
	}
	bundle, err := issuer.Issue("shelly.example.test", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateBundle(bundle, ca, "different.example.test", now); err == nil {
		t.Fatal("accepted wrong hostname")
	}
	wrongCA, _ := newTestCA(t, now, now.Add(4*365*24*time.Hour))
	if err := ValidateBundle(bundle, wrongCA, "shelly.example.test", now); err == nil {
		t.Fatal("accepted wrong root CA")
	}
	otherBundle, err := issuer.Issue("shelly.example.test", now)
	if err != nil {
		t.Fatal(err)
	}
	otherBundle.PrivateKeyPEM = bundle.PrivateKeyPEM
	if err := ValidateBundle(otherBundle, ca, "shelly.example.test", now); err == nil {
		t.Fatal("accepted mismatched leaf/key")
	}
}

func TestRefusesCAAboutToExpire(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	ca, key := newTestCA(t, now, now.Add(2*24*time.Hour))
	issuer, err := NewIssuer(ca, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Issue("shelly.example.test", now); err == nil {
		t.Fatal("expected CA expiry guard")
	}
}

func TestIssuerUsesDistinctSerialAndServerKeys(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	ca, key := newTestCA(t, now, now.Add(4*365*24*time.Hour))
	issuer, _ := NewIssuer(ca, key)
	first, err := issuer.Issue("shelly.example.test", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := issuer.Issue("shelly.example.test", now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Leaf.SerialNumber.Cmp(second.Leaf.SerialNumber) == 0 {
		t.Fatal("reused serial number")
	}
	if string(first.PrivateKeyPEM) == string(second.PrivateKeyPEM) {
		t.Fatal("reused device private key")
	}
}
