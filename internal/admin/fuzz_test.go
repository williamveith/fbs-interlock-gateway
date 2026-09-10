package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
)

const maxAdminFuzzInputBytes = 64 << 10

// FuzzAdminConfigPut exercises the externally controlled JSON accepted by
// PUT /api/config. It intentionally uses the in-memory fake store from
// server_test.go, so fuzzing cannot modify a real configuration or contact a
// Shelly device.
func FuzzAdminConfigPut(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"bind":"127.0.0.1","defaults":{},"tools":[]}`),
		[]byte(`{"bind":" 127.0.0.1 ","defaults":{"timeout_ms":3000,"safe_state_on_error":"off"},"tools":[{"interlock_name":" EQU-TEST-01 ","ip":" 192.0.2.10 ","protocol":" HTTPS ","port":8081,"switch_id":0,"enabled":true}]}`),
		[]byte(`{"bind":"127.0.0.1","defaults":{},"tools":[],"unknown":true}`),
		[]byte(`{"bind":"127.0.0.1","defaults":{},"tools":[]} {}`),
		[]byte(`{"bind":`),
		[]byte(`null`),
		[]byte(`[]`),
		{},
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > maxAdminFuzzInputBytes {
			t.Skip()
		}

		store := &fakeConfigStore{}
		server := newTestAdminServer(
			"127.0.0.1:0",
			store,
			fakeStatusClient{},
			nil,
		)

		request := httptest.NewRequest(
			http.MethodPut,
			"/api/config",
			bytes.NewReader(body),
		)
		response := httptest.NewRecorder()

		server.handleConfig(response, request)

		if got := response.Header().Get("Cache-Control"); got == "" {
			t.Fatal("admin config response omitted Cache-Control")
		}
		if got := response.Header().Get("Pragma"); got == "" {
			t.Fatal("admin config response omitted Pragma")
		}

		switch response.Code {
		case http.StatusOK:
			if store.updateCalls != 1 {
				t.Fatalf(
					"successful PUT called UpdateConfig %d times, want 1",
					store.updateCalls,
				)
			}

			var result map[string]bool
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatalf("successful PUT returned invalid JSON: %v", err)
			}
			if !result["saved"] || !result["restart_required"] {
				t.Fatalf("unexpected successful PUT response: %#v", result)
			}

		case http.StatusBadRequest:
			if store.updateCalls != 0 {
				t.Fatalf(
					"rejected JSON called UpdateConfig %d times, want 0",
					store.updateCalls,
				)
			}

		default:
			t.Fatalf(
				"PUT /api/config returned unexpected status %d for body %q",
				response.Code,
				body,
			)
		}
	})
}

// FuzzAdminPasswordSemantics protects the most security-sensitive part of the
// admin config transformation: an omitted/blank password preserves an existing
// secret by normalized tool name first and then by port, while an explicit new
// password replaces it and clear_password always removes it.
func FuzzAdminPasswordSemantics(f *testing.F) {
	f.Add(
		"EQU-TEST-01",
		" equ-test-01 ",
		"old-password",
		"",
		8081,
		8082,
		true,
		false,
		false,
	)
	f.Add(
		"OLD-NAME",
		"NEW-NAME",
		"old-password",
		" new-password ",
		8081,
		8082,
		true,
		true,
		false,
	)
	f.Add(
		"OLD-NAME",
		"NEW-NAME",
		"old-password",
		"replacement",
		8081,
		8081,
		true,
		true,
		true,
	)
	f.Add(
		"OLD-NAME",
		"NEW-NAME",
		"",
		"",
		8081,
		8081,
		false,
		false,
		false,
	)

	f.Fuzz(func(
		t *testing.T,
		currentName string,
		incomingName string,
		currentPassword string,
		incomingPassword string,
		currentPort int,
		incomingPort int,
		currentPasswordSet bool,
		sendPassword bool,
		clearPassword bool,
	) {
		if len(currentName)+len(incomingName)+
			len(currentPassword)+len(incomingPassword) > maxAdminFuzzInputBytes {
			t.Skip()
		}

		var storedPassword *string
		if currentPasswordSet {
			storedPassword = &currentPassword
		}

		current := config.Config{
			Tools: []config.Tool{
				{
					InterlockName: currentName,
					Port:          currentPort,
					Password:      storedPassword,
				},
			},
		}

		var suppliedPassword *string
		if sendPassword {
			suppliedPassword = &incomingPassword
		}

		request := adminConfigRequest{
			Bind: " 127.0.0.1:18090 ",
			Tools: []adminToolRequest{
				{
					InterlockName: incomingName,
					IP:            " 192.0.2.10 ",
					Protocol:      " HTTPS ",
					Port:          incomingPort,
					Password:      suppliedPassword,
					ClearPassword: clearPassword,
					Enabled:       true,
				},
			},
		}

		updated := buildUpdatedConfig(current, request)
		if len(updated.Tools) != 1 {
			t.Fatalf("updated tool count = %d, want 1", len(updated.Tools))
		}

		got := updated.Tools[0].Password
		wantSet, want := expectedAdminPassword(
			currentName,
			incomingName,
			currentPassword,
			incomingPassword,
			currentPort,
			incomingPort,
			currentPasswordSet,
			sendPassword,
			clearPassword,
		)

		if !wantSet {
			if got != nil {
				t.Fatalf("password = %q, want nil", *got)
			}
			return
		}

		if got == nil {
			t.Fatalf("password = nil, want %q", want)
		}
		if *got != want {
			t.Fatalf("password = %q, want %q", *got, want)
		}
	})
}

func expectedAdminPassword(
	currentName string,
	incomingName string,
	currentPassword string,
	incomingPassword string,
	currentPort int,
	incomingPort int,
	currentPasswordSet bool,
	sendPassword bool,
	clearPassword bool,
) (bool, string) {
	if clearPassword {
		return false, ""
	}

	if sendPassword {
		trimmed := strings.TrimSpace(incomingPassword)
		if trimmed != "" {
			return true, trimmed
		}
	}

	normalizedIncomingName := strings.ToLower(strings.TrimSpace(incomingName))
	normalizedCurrentName := strings.ToLower(strings.TrimSpace(currentName))
	preserveByName := normalizedIncomingName != "" &&
		normalizedIncomingName == normalizedCurrentName
	preserveByPort := incomingPort == currentPort

	if currentPasswordSet && (preserveByName || preserveByPort) {
		return true, currentPassword
	}

	return false, ""
}

// FuzzAdminRequestProtection composes the same security middleware order used
// by Run and checks that arbitrary Host/Origin/Sec-Fetch-Site values cannot
// reach the protected handler unless they satisfy the admin request policy.
func FuzzAdminRequestProtection(f *testing.F) {
	f.Add("GET", "localhost:18090", "", "")
	f.Add("PUT", "localhost:18090", "http://localhost:18090", "same-origin")
	f.Add("POST", "127.0.0.1:18090", "https://evil.example", "cross-site")
	f.Add("DELETE", "evil.example:18090", "", "")
	f.Add("OPTIONS", "LOCALHOST:18090", "https://evil.example", "cross-site")

	f.Fuzz(func(
		t *testing.T,
		method string,
		host string,
		origin string,
		secFetchSite string,
	) {
		if len(method)+len(host)+len(origin)+len(secFetchSite) >
			maxAdminFuzzInputBytes {
			t.Skip()
		}

		server := &Server{}
		reached := false

		leaf := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusNoContent)
		})

		var handler http.Handler = leaf
		handler = server.crossSiteProtectionMiddleware(handler)
		handler = hostValidationMiddleware(handler)
		handler = server.securityHeadersMiddleware(handler)

		request := &http.Request{
			Method: method,
			URL: &url.URL{
				Path: "/api/config",
			},
			Host:   host,
			Header: make(http.Header),
			Body:   http.NoBody,
		}
		request.Header.Set("Origin", origin)
		request.Header.Set("Sec-Fetch-Site", secFetchSite)

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		wantReached := adminFuzzRequestAllowed(
			method,
			host,
			origin,
			secFetchSite,
		)

		if reached != wantReached {
			t.Fatalf(
				"handler reached = %v, want %v (method=%q host=%q origin=%q Sec-Fetch-Site=%q status=%d)",
				reached,
				wantReached,
				method,
				host,
				origin,
				secFetchSite,
				response.Code,
			)
		}

		if reached && response.Code != http.StatusNoContent {
			t.Fatalf("allowed request status = %d, want %d", response.Code, http.StatusNoContent)
		}
		if !reached && response.Code != http.StatusForbidden {
			t.Fatalf("rejected request status = %d, want %d", response.Code, http.StatusForbidden)
		}

		for _, header := range []string{
			"X-Content-Type-Options",
			"X-Frame-Options",
			"Referrer-Policy",
			"Permissions-Policy",
			"Content-Security-Policy",
		} {
			if response.Header().Get(header) == "" {
				t.Fatalf("security response omitted %s", header)
			}
		}
	})
}

func adminFuzzRequestAllowed(
	method string,
	host string,
	origin string,
	secFetchSite string,
) bool {
	normalizedHost := strings.ToLower(strings.TrimSpace(host))
	if normalizedHost != "127.0.0.1:18090" &&
		normalizedHost != "localhost:18090" {
		return false
	}

	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}

	if strings.EqualFold(secFetchSite, "cross-site") {
		return false
	}

	trimmedOrigin := strings.TrimSpace(origin)
	if trimmedOrigin == "" {
		return true
	}

	parsedOrigin, err := url.Parse(trimmedOrigin)
	if err != nil {
		return false
	}
	if parsedOrigin.Scheme != "http" && parsedOrigin.Scheme != "https" {
		return false
	}

	return strings.EqualFold(parsedOrigin.Host, host)
}
