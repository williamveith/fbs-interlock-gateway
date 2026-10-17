// Package pivsigner exposes the existing YubiKey PIV 9C CA key as a
// crypto.Signer WITHOUT CGO. Each Sign launches the installed OpenSC
// pkcs11-tool command, which loads Yubico's native YKCS11 PKCS#11 module.
// The private CA key never enters the Go process.
package pivsigner

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// YKCS11 object ID 02 maps to YubiKey PIV signing slot 9C.
	signingKeyID       = "02"
	pinEnvName         = "YKCS11_PIV_PIN"
	defaultSignTimeout = 30 * time.Second
)

var ErrClosed = errors.New("PIV signer is closed")

// Options identifies the installed executable, YKCS11 library and the already
// deployed CA identity. ModulePath is a shared-library path, NOT an executable.
// ToolPath names the installed pkcs11-tool executable (typically on PATH).
// SlotID is optional; specify a numeric PKCS#11 slot ID if multiple tokens are
// present (it is NOT the PIV slot number 9C).
// PINs are intentionally not part of Options or persistent gateway config.
type Options struct {
	ToolPath      string
	ModulePath    string
	SlotID        string
	CACertificate *x509.Certificate
	Timeout       time.Duration
}

// Signer fulfills crypto.Signer using a new pkcs11-tool subprocess per call.
// Since each invocation opens a new PKCS#11 session, PIN policy ONCE does not
// persist across calls. The PIN must be supplied securely to each subprocess.
// Sign and Close are serialized; no background attempts or automatic retries.
type Signer struct {
	mu     sync.Mutex
	opts   Options
	pub    *ecdsa.PublicKey
	pin    string
	closed bool
}

var _ crypto.Signer = (*Signer)(nil)

// Open validates configuration and makes exactly ONE hardware-backed trial
// signature to prove that PIV 9C corresponds to CACertificate. It never
// imports, generates, replaces or deletes any YubiKey key or certificate.
// pin is passed only in the subprocess environment, not as a CLI argument.
// The caller must provide it from a protected runtime source; do not store
// it in gateway YAML, SQLite, command arguments, or log messages.
func Open(o Options, pin string) (*Signer, error) {
	pub, err := validateOptions(&o)
	if err != nil {
		return nil, err
	}
	if pin == "" {
		return nil, errors.New("PIV PIN is required; no PIN attempt made")
	}
	s := &Signer{opts: o, pub: pub, pin: pin}

	// Verify physical key identity, not merely the public key copied from
	// the certificate. One signing attempt; never retry a failed PIN.
	var challenge [32]byte
	if _, err := io.ReadFull(rand.Reader, challenge[:]); err != nil {
		return nil, fmt.Errorf("make PIV signer challenge: %w", err)
	}
	digest := sha256.Sum256(challenge[:])
	if _, err := s.Sign(rand.Reader, digest[:], crypto.SHA256); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("verify PIV 9C signer: %w", err)
	}
	return s, nil
}

func validateOptions(o *Options) (*ecdsa.PublicKey, error) {
	if o.CACertificate == nil || !o.CACertificate.IsCA || !o.CACertificate.BasicConstraintsValid || o.CACertificate.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, errors.New("valid issuing CA certificate is required")
	}
	pub, ok := o.CACertificate.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, errors.New("PIV signing requires an ECDSA P-256 issuing CA")
	}
	// ECDH validates the public point without accessing deprecated X/Y
	// coordinates or using the unsafe low-level elliptic.Curve API.
	if _, err := pub.ECDH(); err != nil {
		return nil, fmt.Errorf("invalid P-256 CA public key: %w", err)
	}
	// Own an independent public-key copy: callers may retain or modify the
	// original certificate's key after Open returns.
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("encode P-256 CA public key: %w", err)
	}
	parsed, err := x509.ParsePKIXPublicKey(pubDER)
	if err != nil {
		return nil, fmt.Errorf("parse P-256 CA public key: %w", err)
	}
	pub = parsed.(*ecdsa.PublicKey)
	if o.ToolPath == "" {
		o.ToolPath = "pkcs11-tool"
	}
	if strings.TrimSpace(o.ModulePath) == "" || !filepath.IsAbs(o.ModulePath) {
		return nil, errors.New("YKCS11 module path must be absolute")
	}
	if strings.TrimSpace(o.SlotID) != o.SlotID || strings.HasPrefix(o.SlotID, "-") {
		return nil, errors.New("invalid PKCS#11 slot ID")
	}
	if o.Timeout == 0 {
		o.Timeout = defaultSignTimeout
	}
	if o.Timeout < 0 {
		return nil, errors.New("signing timeout must be positive")
	}
	return pub, nil
}

// Public returns the CA public key. Open has verified that the hardware key
// signs messages verifiable under that key; each Sign verifies again.
func (s *Signer) Public() crypto.PublicKey {
	if s == nil || s.pub == nil {
		return nil
	}
	// A fresh X.509 round-trip prevents callers from modifying the key used
	// internally to verify every hardware-generated signature.
	der, err := x509.MarshalPKIXPublicKey(s.pub)
	if err != nil {
		return nil
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil
	}
	return key
}

// Sign accepts the PREHASHED SHA-256 digest from crypto/x509. It passes those
// exact 32 bytes to pkcs11-tool's CKM_ECDSA mechanism (NO second hash), then
// returns the CLI's ASN.1 DER signature to x509.CreateCertificate.
func (s *Signer) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if s == nil {
		return nil, ErrClosed
	}
	if opts == nil || opts.HashFunc() != crypto.SHA256 || len(digest) != sha256.Size {
		return nil, errors.New("PIV 9C requires an already-hashed 32-byte SHA-256 digest")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.opts.Timeout)
	defer cancel()

	args := []string{
		"--module", s.opts.ModulePath,
		"--sign", "--id", signingKeyID,
		"--mechanism", "ECDSA",
		"--signature-format", "openssl", // DER ECDSA-Sig-Value, not raw R||S
		"--pin", "env:" + pinEnvName,
	}
	if s.opts.SlotID != "" {
		args = append(args, "--slot", s.opts.SlotID)
	}
	cmd := exec.CommandContext(ctx, s.opts.ToolPath, args...)
	cmd.Stdin = bytes.NewReader(digest) // pkcs11-tool reads input from stdin
	// Avoid putting a secret in argv, logs or ordinary persistent config.
	// Other processes with sufficiently privileged access may still inspect
	// subprocess environment: treat the signer process as privileged.
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, pinEnvName+"=") {
			env = append(env, entry)
		}
	}
	cmd.Env = append(env, pinEnvName+"="+s.pin)
	der, err := cmd.Output() // stdout is binary DER; stderr is diagnostics
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("YKCS11 signing timed out or was canceled: %w", ctx.Err())
		}
		// Do not propagate raw stderr: libraries can print sensitive details.
		return nil, fmt.Errorf("pkcs11-tool signing failed (no automatic PIN retry): %w", err)
	}
	if err := validateDER(der); err != nil {
		return nil, fmt.Errorf("invalid YKCS11 signature: %w", err)
	}
	if !ecdsa.VerifyASN1(s.pub, digest, der) {
		return nil, errors.New("PIV 9C signature does not match the configured CA certificate")
	}
	return der, nil
}

func validateDER(der []byte) error {
	if len(der) == 0 || len(der) > 80 {
		return fmt.Errorf("unexpected DER signature length %d", len(der))
	}
	var sig struct{ R, S *big.Int }
	rest, err := asn1.Unmarshal(der, &sig)
	if err != nil || len(rest) != 0 || sig.R == nil || sig.S == nil || sig.R.Sign() <= 0 || sig.S.Sign() <= 0 {
		return errors.New("not a valid DER ECDSA signature")
	}
	return nil
}

// Close prevents future signatures and discards the signer's PIN reference.
// There is no persistent PKCS#11 session or subprocess to close.
func (s *Signer) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.pin = ""
	return nil
}
