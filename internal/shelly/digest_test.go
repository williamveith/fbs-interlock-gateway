package shelly

import (
	"strings"
	"testing"
)

func TestParseDigestChallenge(t *testing.T) {
	challenge := `Digest realm="shelly,device", nonce="abc123", algorithm=SHA-256, qop="auth,auth-int", opaque="xyz"`
	got := parseDigestChallenge(challenge)

	want := map[string]string{
		"realm":     "shelly,device",
		"nonce":     "abc123",
		"algorithm": "SHA-256",
		"qop":       "auth,auth-int",
		"opaque":    "xyz",
	}

	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %q, want %q", key, got[key], value)
		}
	}
}

func TestBuildDigestAuthHeader(t *testing.T) {
	header, err := buildDigestAuthHeader(
		"GET",
		"/rpc/Switch.GetStatus?id=0",
		"admin",
		"password",
		`Digest realm="shelly", nonce="nonce", algorithm=SHA-256, qop="auth"`,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, part := range []string{
		`Digest username="admin"`,
		`realm="shelly"`,
		`nonce="nonce"`,
		`algorithm=SHA-256`,
		`qop=auth`,
		`nc=00000001`,
		`response="`,
		`cnonce="`,
	} {
		if !strings.Contains(header, part) {
			t.Fatalf("header %q does not contain %q", header, part)
		}
	}
}

func TestBuildDigestAuthHeaderRejectsUnsupportedAlgorithm(t *testing.T) {
	_, err := buildDigestAuthHeader(
		"GET",
		"/rpc/Switch.GetStatus?id=0",
		"admin",
		"password",
		`Digest realm="shelly", nonce="nonce", algorithm=MD5`,
	)
	if err == nil {
		t.Fatal("expected unsupported-algorithm error")
	}
}
