package shelly

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func FuzzDigestChallenge(f *testing.F) {
	f.Add(
		`Digest realm="shelly", nonce="abc123", algorithm=SHA-256, qop="auth"`,
		"GET",
		"/rpc/Switch.GetStatus?id=0",
		"admin",
		"password",
		uint32(0),
	)
	f.Add(
		`Digest realm="test", nonce="123", qop="auth,auth-int", opaque="xyz"`,
		"POST",
		"/rpc/Switch.Set?id=0&on=true",
		"user",
		"secret",
		uint32(1),
	)
	f.Add(
		`Digest realm="legacy", nonce="n"`,
		"GET",
		"/rpc/Shelly.GetStatus",
		"",
		"",
		maxDigestNonceCount-1,
	)
	f.Add(
		`Digest realm="shelly", nonce="n", algorithm=SHA-256, qop="auth"`,
		"GET",
		"/",
		"admin",
		"password",
		maxDigestNonceCount,
	)
	f.Add("", "GET", "/", "admin", "password", uint32(0))
	f.Add("Digest", "", "", "", "", uint32(0))
	f.Add(`Digest realm="`, "GET", "/", "admin", "password", uint32(0))
	f.Add(
		`Digest realm="a,b", nonce="x", algorithm=MD5`,
		"GET",
		"/",
		"admin",
		"password",
		uint32(0),
	)

	f.Fuzz(func(
		t *testing.T,
		challenge string,
		method string,
		uri string,
		username string,
		password string,
		startingNonceCount uint32,
	) {
		if len(challenge) > 8192 ||
			len(method) > 64 ||
			len(uri) > 4096 ||
			len(username) > 2048 ||
			len(password) > 2048 {
			t.Skip()
		}

		// The challenge parser itself is intentionally permissive, but no input
		// should ever make it panic.
		_ = parseDigestChallenge(challenge)

		session, err := newDigestSession(challenge)
		if err != nil {
			return
		}

		if session.realm == "" {
			t.Fatal("accepted digest challenge with empty realm")
		}
		if session.nonce == "" {
			t.Fatal("accepted digest challenge with empty nonce")
		}
		if session.algorithm != "SHA-256" {
			t.Fatalf("session algorithm = %q, want SHA-256", session.algorithm)
		}
		if session.qop != "" && session.qop != "auth" {
			t.Fatalf("session qop = %q, want empty or auth", session.qop)
		}

		if session.qop == "auth" {
			if len(session.cnonce) != 32 {
				t.Fatalf("cnonce length = %d, want 32 hex characters", len(session.cnonce))
			}
			if _, err := hex.DecodeString(session.cnonce); err != nil {
				t.Fatalf("cnonce is not hexadecimal: %q", session.cnonce)
			}
		} else if session.cnonce != "" {
			t.Fatalf("qop-less session unexpectedly has cnonce %q", session.cnonce)
		}

		session.nonceCount = startingNonceCount

		header, err := session.nextAuthorizationHeader(
			method,
			uri,
			username,
			password,
		)

		if startingNonceCount >= maxDigestNonceCount {
			if err == nil {
				t.Fatalf(
					"nonce count %d unexpectedly produced authorization header %q",
					startingNonceCount,
					header,
				)
			}
			if session.nonceCount != startingNonceCount {
				t.Fatalf(
					"exhausted nonce count changed from %d to %d",
					startingNonceCount,
					session.nonceCount,
				)
			}
			return
		}

		if err != nil {
			t.Fatalf("valid digest session failed to build header: %v", err)
		}

		wantNonceCount := startingNonceCount + 1
		if session.nonceCount != wantNonceCount {
			t.Fatalf(
				"nonce count = %d, want %d",
				session.nonceCount,
				wantNonceCount,
			)
		}

		params, err := parseGeneratedDigestAuthorization(header)
		if err != nil {
			t.Fatalf("generated malformed Authorization header %q: %v", header, err)
		}

		assertDigestParam(t, params, "username", username)
		assertDigestParam(t, params, "realm", session.realm)
		assertDigestParam(t, params, "nonce", session.nonce)
		assertDigestParam(t, params, "uri", uri)
		assertDigestParam(t, params, "algorithm", "SHA-256")

		if session.opaque == "" {
			if _, ok := params["opaque"]; ok {
				t.Fatalf("empty opaque unexpectedly emitted in header %q", header)
			}
		} else {
			assertDigestParam(t, params, "opaque", session.opaque)
		}

		ha1 := digestSHA256ForFuzz(username + ":" + session.realm + ":" + password)
		ha2 := digestSHA256ForFuzz(method + ":" + uri)

		var wantResponse string
		if session.qop == "auth" {
			wantNC := fmt.Sprintf("%08x", wantNonceCount)
			assertDigestParam(t, params, "qop", "auth")
			assertDigestParam(t, params, "nc", wantNC)
			assertDigestParam(t, params, "cnonce", session.cnonce)

			wantResponse = digestSHA256ForFuzz(
				ha1 + ":" + session.nonce + ":" + wantNC + ":" +
					session.cnonce + ":auth:" + ha2,
			)
		} else {
			for _, key := range []string{"qop", "nc", "cnonce"} {
				if _, ok := params[key]; ok {
					t.Fatalf("qop-less digest unexpectedly emitted %s in %q", key, header)
				}
			}

			wantResponse = digestSHA256ForFuzz(
				ha1 + ":" + session.nonce + ":" + ha2,
			)
		}

		assertDigestParam(t, params, "response", wantResponse)

		response := params["response"]
		if len(response) != 64 {
			t.Fatalf("response digest length = %d, want 64", len(response))
		}
		if _, err := hex.DecodeString(response); err != nil {
			t.Fatalf("response is not hexadecimal: %q", response)
		}
	})
}

func digestSHA256ForFuzz(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func assertDigestParam(
	t *testing.T,
	params map[string]string,
	key string,
	want string,
) {
	t.Helper()

	got, ok := params[key]
	if !ok {
		t.Fatalf("generated digest header is missing %q", key)
	}
	if got != want {
		t.Fatalf("digest parameter %s = %q, want %q", key, got, want)
	}
}

// parseGeneratedDigestAuthorization is deliberately separate from the
// production challenge parser. It acts as the oracle for the Authorization
// header emitted by nextAuthorizationHeader, including escaped quoted values.
func parseGeneratedDigestAuthorization(header string) (map[string]string, error) {
	if !strings.HasPrefix(header, "Digest ") {
		return nil, fmt.Errorf("missing Digest prefix")
	}

	raw := strings.TrimSpace(strings.TrimPrefix(header, "Digest "))
	parts := splitGeneratedDigestParams(raw)
	params := make(map[string]string, len(parts))

	for _, part := range parts {
		key, rawValue, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("parameter %q has no equals sign", part)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("empty parameter name")
		}
		if _, exists := params[key]; exists {
			return nil, fmt.Errorf("duplicate parameter %q", key)
		}

		value, err := decodeGeneratedDigestValue(strings.TrimSpace(rawValue))
		if err != nil {
			return nil, fmt.Errorf("parameter %s: %w", key, err)
		}
		params[key] = value
	}

	return params, nil
}

func splitGeneratedDigestParams(value string) []string {
	parts := make([]string, 0, 10)
	start := 0
	inQuote := false
	escaped := false

	for i := 0; i < len(value); i++ {
		c := value[i]

		if escaped {
			escaped = false
			continue
		}
		if inQuote && c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			inQuote = !inQuote
			continue
		}
		if c == ',' && !inQuote {
			parts = append(parts, strings.TrimSpace(value[start:i]))
			start = i + 1
		}
	}

	parts = append(parts, strings.TrimSpace(value[start:]))
	return parts
}

func decodeGeneratedDigestValue(value string) (string, error) {
	if value == "" || value[0] != '"' {
		return value, nil
	}
	if len(value) < 2 || value[len(value)-1] != '"' {
		return "", fmt.Errorf("unterminated quoted value")
	}

	value = value[1 : len(value)-1]
	decoded := make([]byte, 0, len(value))

	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			decoded = append(decoded, value[i])
			continue
		}

		i++
		if i >= len(value) {
			return "", fmt.Errorf("trailing escape")
		}
		decoded = append(decoded, value[i])
	}

	return string(decoded), nil
}
