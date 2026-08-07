package shelly

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	maxDigestNonceCount uint32 = 30000
	digestSessionMaxAge        = 55 * time.Minute
)

type digestSession struct {
	realm      string
	nonce      string
	algorithm  string
	qop        string
	opaque     string
	cnonce     string
	nonceCount uint32
	createdAt  time.Time
}

func newDigestSession(challenge string) (*digestSession, error) {
	params := parseDigestChallenge(challenge)

	realm := params["realm"]
	nonce := params["nonce"]
	algorithm := params["algorithm"]
	qopRaw := params["qop"]

	if realm == "" {
		return nil, fmt.Errorf("digest challenge missing realm")
	}
	if nonce == "" {
		return nil, fmt.Errorf("digest challenge missing nonce")
	}

	if algorithm == "" {
		algorithm = "SHA-256"
	}
	if !strings.EqualFold(algorithm, "SHA-256") {
		return nil, fmt.Errorf("unsupported digest algorithm %q", algorithm)
	}

	qop := chooseDigestQOP(qopRaw)
	if strings.TrimSpace(qopRaw) != "" && qop == "" {
		return nil, fmt.Errorf("unsupported digest qop %q", qopRaw)
	}

	cnonce := ""
	if qop == "auth" {
		var err error
		cnonce, err = randomHex(16)
		if err != nil {
			return nil, err
		}
	}

	return &digestSession{
		realm:     realm,
		nonce:     nonce,
		algorithm: "SHA-256",
		qop:       qop,
		opaque:    params["opaque"],
		cnonce:    cnonce,
		createdAt: time.Now(),
	}, nil
}

func (s *digestSession) expired(now time.Time) bool {
	return s == nil ||
		now.Sub(s.createdAt) >= digestSessionMaxAge ||
		s.nonceCount >= maxDigestNonceCount
}

func (s *digestSession) nextAuthorizationHeader(
	method string,
	uri string,
	username string,
	password string,
) (string, error) {
	if s == nil {
		return "", fmt.Errorf("digest session is nil")
	}
	if s.nonceCount >= maxDigestNonceCount {
		return "", fmt.Errorf("digest nonce count exhausted")
	}

	s.nonceCount++
	nc := fmt.Sprintf("%08x", s.nonceCount)

	ha1 := sha256Hex(username + ":" + s.realm + ":" + password)
	ha2 := sha256Hex(method + ":" + uri)

	if s.qop == "auth" {
		response := sha256Hex(
			ha1 + ":" + s.nonce + ":" + nc + ":" + s.cnonce + ":" + s.qop + ":" + ha2,
		)

		header := fmt.Sprintf(
			`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=SHA-256, response="%s", qop=auth, nc=%s, cnonce="%s"`,
			escapeDigest(username),
			escapeDigest(s.realm),
			escapeDigest(s.nonce),
			escapeDigest(uri),
			response,
			nc,
			escapeDigest(s.cnonce),
		)

		if s.opaque != "" {
			header += fmt.Sprintf(`, opaque="%s"`, escapeDigest(s.opaque))
		}

		return header, nil
	}

	response := sha256Hex(ha1 + ":" + s.nonce + ":" + ha2)

	header := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=SHA-256, response="%s"`,
		escapeDigest(username),
		escapeDigest(s.realm),
		escapeDigest(s.nonce),
		escapeDigest(uri),
		response,
	)

	if s.opaque != "" {
		header += fmt.Sprintf(`, opaque="%s"`, escapeDigest(s.opaque))
	}

	return header, nil
}

// buildDigestAuthHeader remains as a one-request helper for tests and callers
// that already have a challenge. Client requests use a cached digestSession so
// the nonce count can increase across calls.
func buildDigestAuthHeader(method, uri, username, password, challenge string) (string, error) {
	session, err := newDigestSession(challenge)
	if err != nil {
		return "", err
	}

	return session.nextAuthorizationHeader(method, uri, username, password)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}

func chooseDigestQOP(qopRaw string) string {
	for _, part := range strings.Split(qopRaw, ",") {
		if strings.EqualFold(strings.TrimSpace(part), "auth") {
			return "auth"
		}
	}

	return ""
}

func parseDigestChallenge(header string) map[string]string {
	result := map[string]string{}

	header = strings.TrimSpace(header)

	if strings.HasPrefix(strings.ToLower(header), "digest ") {
		header = strings.TrimSpace(header[len("digest "):])
	}

	for _, part := range splitDigestHeader(header) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}

		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"`)

		result[key] = value
	}

	return result
}

func splitDigestHeader(s string) []string {
	parts := make([]string, 0, strings.Count(s, ",")+1)
	var b strings.Builder

	inQuote := false
	escaped := false

	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false

		case r == '\\':
			b.WriteRune(r)
			escaped = true

		case r == '"':
			b.WriteRune(r)
			inQuote = !inQuote

		case r == ',' && !inQuote:
			parts = append(parts, strings.TrimSpace(b.String()))
			b.Reset()

		default:
			b.WriteRune(r)
		}
	}

	if strings.TrimSpace(b.String()) != "" {
		parts = append(parts, strings.TrimSpace(b.String()))
	}

	return parts
}

func escapeDigest(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
