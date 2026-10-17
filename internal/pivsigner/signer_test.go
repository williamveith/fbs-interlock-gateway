package pivsigner

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/certmanager"
)

// This test binary doubles as a fake CLI launched via exec.Command. It does
// not load YKCS11 or perform any real token operations.
func init() {
	if os.Getenv("YKCS11_TEST_HELPER") != "1" {
		return
	}
	args := strings.Join(os.Args[1:], " ")
	for _, required := range []string{
		"--module /mock/libykcs11.so", "--sign", "--id 02",
		"--mechanism ECDSA", "--signature-format openssl",
		"--pin env:YKCS11_PIV_PIN",
	} {
		if !strings.Contains(args, required) {
			os.Exit(24)
		}
	}
	if os.Getenv("YKCS11_PIV_PIN") != "test-pin-not-real" {
		os.Exit(25)
	}
	if path := os.Getenv("YKCS11_TEST_COUNT_FILE"); path != "" {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
		if err != nil {
			os.Exit(28)
		}
		_, _ = f.Write([]byte("1\n"))
		_ = f.Close()
	}
	if os.Getenv("YKCS11_TEST_FAIL") == "1" {
		os.Exit(26)
	}
	if os.Getenv("YKCS11_TEST_SLEEP") == "1" {
		time.Sleep(time.Second)
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil || len(input) != 32 {
		os.Exit(27)
	}
	if os.Getenv("YKCS11_TEST_BAD_DER") == "1" {
		_, _ = os.Stdout.Write([]byte("not-der"))
		os.Exit(0)
	}
	// Public test-only key: private scalar = 1 (the P-256 generator point).
	key := publicTestPrivateKey(1)
	if os.Getenv("YKCS11_TEST_WRONG_KEY") == "1" {
		key = publicTestPrivateKey(2)
	}
	der, err := ecdsa.SignASN1(rand.Reader, key, input)
	if err != nil {
		os.Exit(29)
	}
	_, _ = os.Stdout.Write(der)
	os.Exit(0)
}

// Fixed, public test-only P-256 private keys (scalars 1 and 2). The helper
// subprocess and its parent must use the same identity, so generating a
// random key separately in each process would not work here.
const testPrivateKey1PEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABoAoGCCqGSM49
AwEHoUQDQgAEaxfR8uEsQkf4vOblY6RA8ncDfYEt6zOg9KE5RdiYwpZP40Li/hp/
m47n60p8D54WK84zV2sxXs7LtkBoN79R9Q==
-----END EC PRIVATE KEY-----`

const testPrivateKey2PEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACoAoGCCqGSM49
AwEHoUQDQgAEfPJ7GI0DT36KUjgDBLUaw8CJaeJ38hs1pgtI/EdmmXgHd1UQ247Q
QCk9msafdDDbun2t5jzpgimeBLedInhz0Q==
-----END EC PRIVATE KEY-----`

func publicTestPrivateKey(id int) *ecdsa.PrivateKey {
	fixture := testPrivateKey1PEM
	if id == 2 {
		fixture = testPrivateKey2PEM
	}
	block, _ := pem.Decode([]byte(fixture))
	if block == nil {
		panic("invalid test private key PEM")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	return key
}

func testCert(t *testing.T, key *ecdsa.PrivateKey) *x509.Certificate {
	t.Helper()
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "test server CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func helperOptions(t *testing.T) Options {
	t.Helper()
	t.Setenv("YKCS11_TEST_HELPER", "1")
	return Options{
		ToolPath: os.Args[0], ModulePath: "/mock/libykcs11.so",
		CACertificate: testCert(t, publicTestPrivateKey(1)), Timeout: 3 * time.Second,
	}
}

func TestExecSignerUsesExternalCLIAndSignsDigest(t *testing.T) {
	o := helperOptions(t)
	signer, err := Open(o, "test-pin-not-real")
	if err != nil {
		t.Fatal(err)
	}
	defer signer.Close()
	digest := sha256.Sum256([]byte("test certificate signature"))
	der, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !ecdsa.VerifyASN1(o.CACertificate.PublicKey.(*ecdsa.PublicKey), digest[:], der) {
		t.Fatal("external signature verification failed")
	}
	if _, err := signer.Sign(nil, digest[:20], crypto.SHA256); err == nil {
		t.Fatal("accepted short digest")
	}
	if _, err := signer.Sign(nil, digest[:], crypto.SHA512); err == nil {
		t.Fatal("accepted wrong hash")
	}
	signer.Close()
	if _, err := signer.Sign(nil, digest[:], crypto.SHA256); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed signer error: %v", err)
	}
}

func TestPublicReturnsIndependentCopy(t *testing.T) {
	o := helperOptions(t)
	signer, err := Open(o, "test-pin-not-real")
	if err != nil {
		t.Fatal(err)
	}
	defer signer.Close()

	pub := signer.Public().(*ecdsa.PublicKey)
	original := o.CACertificate.PublicKey.(*ecdsa.PublicKey)
	if !pub.Equal(original) {
		t.Fatal("public key does not match issuing CA")
	}

	// Replacing the exported key object must not change the internal key.
	*pub = publicTestPrivateKey(2).PublicKey
	if !signer.Public().(*ecdsa.PublicKey).Equal(original) {
		t.Fatal("caller mutated the signer's internal public key")
	}

	digest := sha256.Sum256([]byte("public key defensive copy"))
	if _, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256); err != nil {
		t.Fatalf("caller mutation interfered with signing: %v", err)
	}
}

func TestOpenRejectsDifferentHardwareKey(t *testing.T) {
	o := helperOptions(t)
	t.Setenv("YKCS11_TEST_WRONG_KEY", "1")
	if _, err := Open(o, "test-pin-not-real"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected wrong-key rejection, got %v", err)
	}
}

func TestFailedPINIsNotRetried(t *testing.T) {
	o := helperOptions(t)
	path := t.TempDir() + "/calls"
	t.Setenv("YKCS11_TEST_COUNT_FILE", path)
	t.Setenv("YKCS11_TEST_FAIL", "1")
	_, err := Open(o, "test-pin-not-real")
	if err == nil {
		t.Fatal("expected signing failure")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte("1\n")) {
		t.Fatalf("expected exactly one CLI invocation, got %q", data)
	}
}

func TestInvalidDERSignatureFails(t *testing.T) {
	o := helperOptions(t)
	t.Setenv("YKCS11_TEST_BAD_DER", "1")
	if _, err := Open(o, "test-pin-not-real"); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected DER validation failure, got %v", err)
	}
}

func TestSigningTimesOut(t *testing.T) {
	o := helperOptions(t)
	o.Timeout = 20 * time.Millisecond
	t.Setenv("YKCS11_TEST_SLEEP", "1")
	if _, err := Open(o, "test-pin-not-real"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestMissingPINDoesNotInvokeCLI(t *testing.T) {
	o := helperOptions(t)
	if _, err := Open(o, ""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected missing PIN error, got %v", err)
	}
}

func TestExternalSignatureSignsFullCertificate(t *testing.T) {
	o := helperOptions(t)
	signer, err := Open(o, "test-pin-not-real")
	if err != nil {
		t.Fatal(err)
	}
	defer signer.Close()
	issuer, err := certmanager.NewIssuer(o.CACertificate, signer)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	bundle, err := issuer.Issue("a085e3b5325c.dynamic.utexas.edu", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := certmanager.ValidateBundle(bundle, o.CACertificate, "a085e3b5325c.dynamic.utexas.edu", now); err != nil {
		t.Fatalf("full certificate verification failed: %v", err)
	}
}
