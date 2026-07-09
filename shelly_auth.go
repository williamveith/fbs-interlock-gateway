package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (g *Gateway) doShellyGET(tool Tool, url string) (*http.Response, error) {
	// If no password is configured, preserve the old unauthenticated behavior.
	if tool.Password == nil || strings.TrimSpace(*tool.Password) == "" {
		return g.client.Get(url)
	}

	username := "admin"
	if tool.Username != nil && strings.TrimSpace(*tool.Username) != "" {
		username = strings.TrimSpace(*tool.Username)
	}

	password := *tool.Password

	// First request intentionally has no Authorization header.
	// Shelly responds with 401 and a WWW-Authenticate digest challenge.
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}

	// If auth is not enabled yet, Shelly may return 200 immediately.
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	challenge := resp.Header.Get("WWW-Authenticate")

	// Drain and close the 401 body before retrying.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()

	if challenge == "" {
		return nil, fmt.Errorf("shelly returned 401 but no WWW-Authenticate header")
	}

	authHeader, err := buildShellyDigestAuthHeader(
		http.MethodGet,
		req.URL.RequestURI(),
		username,
		password,
		challenge,
	)
	if err != nil {
		return nil, err
	}

	retryReq, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	retryReq.Header.Set("Authorization", authHeader)

	return g.client.Do(retryReq)
}

func buildShellyDigestAuthHeader(method, uri, username, password, challenge string) (string, error) {
	params := parseDigestChallenge(challenge)

	realm := params["realm"]
	nonce := params["nonce"]
	algorithm := params["algorithm"]
	qopRaw := params["qop"]
	opaque := params["opaque"]

	if realm == "" {
		return "", fmt.Errorf("digest challenge missing realm")
	}
	if nonce == "" {
		return "", fmt.Errorf("digest challenge missing nonce")
	}

	if algorithm == "" {
		algorithm = "SHA-256"
	}

	if !strings.EqualFold(algorithm, "SHA-256") {
		return "", fmt.Errorf("unsupported digest algorithm %q", algorithm)
	}

	qop := chooseDigestQOP(qopRaw)

	ha1 := sha256Hex(username + ":" + realm + ":" + password)
	ha2 := sha256Hex(method + ":" + uri)

	response := ""

	if qop == "auth" {
		nc := "00000001"

		cnonce, err := randomHex(16)
		if err != nil {
			return "", err
		}

		response = sha256Hex(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)

		header := fmt.Sprintf(
			`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=SHA-256, response="%s", qop=auth, nc=%s, cnonce="%s"`,
			escapeDigest(username),
			escapeDigest(realm),
			escapeDigest(nonce),
			escapeDigest(uri),
			response,
			nc,
			escapeDigest(cnonce),
		)

		if opaque != "" {
			header += fmt.Sprintf(`, opaque="%s"`, escapeDigest(opaque))
		}

		return header, nil
	}

	// Fallback for older/legacy digest challenges without qop.
	response = sha256Hex(ha1 + ":" + nonce + ":" + ha2)

	header := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=SHA-256, response="%s"`,
		escapeDigest(username),
		escapeDigest(realm),
		escapeDigest(nonce),
		escapeDigest(uri),
		response,
	)

	if opaque != "" {
		header += fmt.Sprintf(`, opaque="%s"`, escapeDigest(opaque))
	}

	return header, nil
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
	var parts []string
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
