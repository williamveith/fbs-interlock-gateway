package updateauth

import (
	"crypto/ed25519"
	"crypto/x509"
	_ "embed"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

//go:embed update-signing-public.pem
var updateSigningPublicKeyPEM []byte

func VerifyUpdateChecksumCommand(args []string) (string, error) {
	flags := flag.NewFlagSet("verify-update-checksum", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	checksumPath := flags.String(
		"checksum",
		"",
		"path to the signed .sha256 file",
	)
	signaturePath := flags.String(
		"signature",
		"",
		"path to the detached Ed25519 signature",
	)
	expectedAsset := flags.String(
		"asset",
		"",
		"expected release asset filename",
	)

	if err := flags.Parse(args); err != nil {
		return "", err
	}

	if flags.NArg() != 0 {
		return "", fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}

	if strings.TrimSpace(*checksumPath) == "" {
		return "", errors.New("--checksum is required")
	}

	if strings.TrimSpace(*signaturePath) == "" {
		return "", errors.New("--signature is required")
	}

	if strings.TrimSpace(*expectedAsset) == "" {
		return "", errors.New("--asset is required")
	}

	publicKey, err := parseUpdateSigningPublicKey()
	if err != nil {
		return "", err
	}

	checksumBytes, err := os.ReadFile(*checksumPath)
	if err != nil {
		return "", fmt.Errorf("read checksum file: %w", err)
	}

	signature, err := os.ReadFile(*signaturePath)
	if err != nil {
		return "", fmt.Errorf("read signature file: %w", err)
	}

	if len(signature) != ed25519.SignatureSize {
		return "", fmt.Errorf(
			"invalid Ed25519 signature length: got %d bytes, want %d",
			len(signature),
			ed25519.SignatureSize,
		)
	}

	if !ed25519.Verify(publicKey, checksumBytes, signature) {
		return "", errors.New("invalid update checksum signature")
	}

	checksum, asset, err := parseSignedChecksum(checksumBytes)
	if err != nil {
		return "", err
	}

	if asset != *expectedAsset {
		return "", fmt.Errorf(
			"signed checksum is for asset %q, expected %q",
			asset,
			*expectedAsset,
		)
	}

	return checksum, nil
}

func parseUpdateSigningPublicKey() (ed25519.PublicKey, error) {
	block, rest := pem.Decode([]byte(updateSigningPublicKeyPEM))
	if block == nil {
		return nil, errors.New("decode embedded update signing public key")
	}

	if strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("embedded update signing public key contains trailing data")
	}

	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf(
			"embedded update signing key has PEM type %q, expected PUBLIC KEY",
			block.Type,
		)
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse embedded update signing public key: %w", err)
	}

	publicKey, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf(
			"embedded update signing public key has type %T, expected Ed25519",
			key,
		)
	}

	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf(
			"invalid Ed25519 public key length: got %d bytes, want %d",
			len(publicKey),
			ed25519.PublicKeySize,
		)
	}

	return publicKey, nil
}

func parseSignedChecksum(data []byte) (string, string, error) {
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", "", errors.New("signed checksum file is empty")
	}

	lines := strings.Split(content, "\n")
	if len(lines) != 1 {
		return "", "", errors.New("signed checksum file must contain exactly one line")
	}

	fields := strings.Fields(lines[0])
	if len(fields) != 2 {
		return "", "", errors.New(
			"signed checksum line must contain exactly a SHA-256 digest and asset filename",
		)
	}

	checksum := strings.ToLower(fields[0])
	decoded, err := hex.DecodeString(checksum)
	if err != nil || len(decoded) != 32 {
		return "", "", errors.New("signed checksum contains an invalid SHA-256 digest")
	}

	asset := strings.TrimPrefix(fields[1], "*")
	if asset == "" {
		return "", "", errors.New("signed checksum contains an empty asset filename")
	}

	if strings.ContainsAny(asset, `/\`) {
		return "", "", errors.New("signed checksum asset must be a base filename")
	}

	return checksum, asset, nil
}
