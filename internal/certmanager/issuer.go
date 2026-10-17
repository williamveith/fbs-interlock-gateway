package certmanager

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	defaultLeafLifetime = 825 * 24 * time.Hour
	caExpiryBuffer      = 24 * time.Hour
	minIssuedLifetime   = 7 * 24 * time.Hour
)

// Issuer uses a crypto.Signer, not a software CA private key. The real signer
// may invoke an installed pkcs11-tool backed by YKCS11. Only the
// replaceable per-Shelly *leaf* key is generated outside the signer.
type Issuer struct {
	ca     *x509.Certificate
	signer crypto.Signer
}

type Bundle struct {
	CertificatePEM []byte
	PrivateKeyPEM  []byte // PRIVATE: do not log, persist casually, or embed in responses
	Leaf           *x509.Certificate
}

func NewIssuer(ca *x509.Certificate, signer crypto.Signer) (*Issuer, error) {
	if ca == nil || signer == nil {
		return nil, errors.New("issuing CA and crypto.Signer are required")
	}
	if !ca.IsCA || !ca.BasicConstraintsValid || ca.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, errors.New("issuer certificate is not authorized to sign certificates")
	}
	caPublic, err := x509.MarshalPKIXPublicKey(ca.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal CA public key: %w", err)
	}
	signerPublic, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil {
		return nil, fmt.Errorf("marshal signer public key: %w", err)
	}
	if !bytes.Equal(caPublic, signerPublic) {
		return nil, errors.New("CA certificate public key does not match configured signer")
	}
	return &Issuer{ca: ca, signer: signer}, nil
}

// Issue creates a new device-specific P-256 key and an X.509 server certificate
// signed by the configured CA. Signing may block on PIN or physical touch;
// NEVER invoke this function on an FBS control request goroutine.
func (i *Issuer) Issue(host string, now time.Time) (*Bundle, error) {
	if i == nil {
		return nil, errors.New("issuer is nil")
	}
	if err := validateDNSHostname(host); err != nil {
		return nil, err
	}
	now = now.UTC()
	if now.Before(i.ca.NotBefore) || !now.Before(i.ca.NotAfter) {
		return nil, errors.New("CA certificate is not valid at issuance time")
	}
	notBefore := now.Add(-5 * time.Minute)
	if notBefore.Before(i.ca.NotBefore) {
		notBefore = i.ca.NotBefore
	}
	notAfter := now.Add(defaultLeafLifetime)
	if limit := i.ca.NotAfter.Add(-caExpiryBuffer); notAfter.After(limit) {
		notAfter = limit
	}
	if notAfter.Sub(now) < minIssuedLifetime {
		return nil, errors.New("issuing CA expires too soon to issue a safe replacement")
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate device key: %w", err)
	}
	// Match the original OpenSSL shelly-server.cnf: subjectKeyIdentifier = hash.
	// An ECDSA P-256 public key's ECDH encoding is the same uncompressed
	// subjectPublicKey octets hashed by OpenSSL for this extension. SHA-1 here
	// is the RFC 5280 identifier construction, not a signature algorithm.
	publicECDH, err := leafKey.PublicKey.ECDH()
	if err != nil {
		return nil, fmt.Errorf("encode device public key for SKI: %w", err)
	}
	ski := sha1.Sum(publicECDH.Bytes())
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: host},
		DNSNames:              []string{host},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		SubjectKeyId:          ski[:],
	}
	// Go's x509.CreateCertificate copies the CA's SubjectKeyId into the
	// leaf AuthorityKeyId, matching authorityKeyIdentifier = keyid,issuer.
	der, err := x509.CreateCertificate(rand.Reader, template, i.ca, &leafKey.PublicKey, i.signer)
	if err != nil {
		return nil, fmt.Errorf("CA signer failed: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse signed certificate: %w", err)
	}
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return nil, fmt.Errorf("marshal device key: %w", err)
	}
	bundle := &Bundle{
		CertificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		PrivateKeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER}),
		Leaf:           leaf,
	}
	if err := ValidateBundle(bundle, i.ca, host, now); err != nil {
		return nil, fmt.Errorf("reject issued bundle before installation: %w", err)
	}
	return bundle, nil
}

// ValidateBundle checks the leaf/key match, the configured CA trust chain,
// hostname SAN, validity, and server-auth purpose before device upload.
func ValidateBundle(bundle *Bundle, ca *x509.Certificate, host string, now time.Time) error {
	if bundle == nil || ca == nil {
		return errors.New("certificate bundle and CA are required")
	}
	if err := validateDNSHostname(host); err != nil {
		return err
	}
	pair, err := tls.X509KeyPair(bundle.CertificatePEM, bundle.PrivateKeyPEM)
	if err != nil {
		return fmt.Errorf("certificate/private key mismatch: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return errors.New("leaf certificate is missing")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse device certificate: %w", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:       roots,
		DNSName:     host,
		CurrentTime: now,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		return fmt.Errorf("verify device certificate: %w", err)
	}
	if leaf.IsCA || !leaf.BasicConstraintsValid || leaf.Subject.CommonName != host {
		return errors.New("device certificate must be a non-CA leaf with the requested common name")
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != host || len(leaf.IPAddresses) != 0 ||
		len(leaf.EmailAddresses) != 0 || len(leaf.URIs) != 0 {
		return errors.New("device certificate must have exactly the requested DNS SAN and no other SANs")
	}
	if leaf.KeyUsage != x509.KeyUsageDigitalSignature ||
		len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		return errors.New("device certificate must have digitalSignature and serverAuth usages")
	}
	if len(leaf.SubjectKeyId) == 0 || len(ca.SubjectKeyId) == 0 || !bytes.Equal(leaf.AuthorityKeyId, ca.SubjectKeyId) {
		return errors.New("device certificate must include matching subject and authority key identifiers")
	}
	return nil
}

func validateDNSHostname(host string) error {
	if host == "" || host != strings.TrimSpace(host) || len(host) > 253 || strings.Contains(host, ":") || strings.Contains(host, "/") || strings.Contains(host, "*") {
		return fmt.Errorf("invalid device DNS hostname %q", host)
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return fmt.Errorf("device hostname must be a full DNS name: %q", host)
	}
	// Original create-shelly-cert.sh only issued for UT Austin DDNS hosts.
	// Do not mint certificates for arbitrary DNS names or IP addresses.
	if !strings.HasSuffix(host, ".dynamic.utexas.edu") || len(labels) < 4 {
		return fmt.Errorf("device hostname must end in .dynamic.utexas.edu: %q", host)
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("invalid device DNS hostname %q", host)
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return fmt.Errorf("invalid device DNS hostname %q", host)
			}
		}
	}
	return nil
}
