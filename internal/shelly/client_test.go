package shelly

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
)

func toolForServer(server *httptest.Server) config.Tool {
	return config.Tool{
		InterlockName: "EQU-TEST-01",
		IP:            strings.TrimPrefix(server.URL, "http://"),
		Port:          8081,
		SwitchID:      0,
		Enabled:       true,
	}
}

func TestClientGetStatusAndSetWithoutAuthentication(t *testing.T) {
	var setValue string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Switch.GetStatus":
			fmt.Fprint(w, `{"id":0,"output":true}`)
		case "/rpc/Switch.Set":
			setValue = r.URL.Query().Get("on")
			fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := toolForServer(server)

	status, err := client.GetStatus(context.Background(), tool)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Output {
		t.Fatal("status output = false, want true")
	}

	if err := client.Set(context.Background(), tool, false); err != nil {
		t.Fatal(err)
	}
	if setValue != "false" {
		t.Fatalf("set value = %q, want false", setValue)
	}
}

func TestClientRetriesWithDigestAuthorization(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="shelly", nonce="nonce", algorithm=SHA-256, qop="auth"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(r.Header.Get("Authorization"), "Digest ") {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		fmt.Fprint(w, `{"id":0,"output":false}`)
	}))
	defer server.Close()

	username := "admin"
	password := "password"
	tool := toolForServer(server)
	tool.Username = &username
	tool.Password = &password

	client := NewClient(time.Second)
	if _, err := client.GetStatus(context.Background(), tool); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestClientRejectsInvalidStatusJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{not-json}`)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	if _, err := client.GetStatus(context.Background(), toolForServer(server)); err == nil {
		t.Fatal("expected JSON decoding error")
	}
}

func TestGetStatusReturnsTypedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad switch id", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := testTool(server)

	_, err := client.GetStatus(context.Background(), tool)
	if err == nil {
		t.Fatal("expected an error")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T: %v", err, err)
	}

	if httpErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", httpErr.StatusCode)
	}

	if !IsHTTPStatus(err, http.StatusBadRequest) {
		t.Fatal("expected IsHTTPStatus to match 400")
	}
}

func TestRateLimitSchedulesSingleReboot(t *testing.T) {
	var rebootRequests atomic.Int32
	rebootSeen := make(chan struct{}, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Switch.GetStatus":
			http.Error(w, "Too many Requests", http.StatusLocked)

		case "/rpc/Shelly.Reboot":
			rebootRequests.Add(1)
			rebootSeen <- struct{}{}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("null"))

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(time.Second)
	client.rebootCooldown = time.Hour
	tool := testTool(server)

	_, err := client.GetStatus(context.Background(), tool)
	if !IsHTTPStatus(err, http.StatusLocked) {
		t.Fatalf("expected HTTP 423, got %v", err)
	}

	select {
	case <-rebootSeen:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reboot request")
	}

	_, err = client.GetStatus(context.Background(), tool)
	if !IsHTTPStatus(err, http.StatusLocked) {
		t.Fatalf("expected second HTTP 423, got %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if got := rebootRequests.Load(); got != 1 {
		t.Fatalf("expected exactly one reboot request, got %d", got)
	}
}

func TestBadRequestDoesNotScheduleReboot(t *testing.T) {
	var rebootRequests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rpc/Shelly.Reboot" {
			rebootRequests.Add(1)
			_, _ = w.Write([]byte("null"))
			return
		}

		http.Error(w, "invalid argument", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := testTool(server)

	_, err := client.GetStatus(context.Background(), tool)
	if !IsHTTPStatus(err, http.StatusBadRequest) {
		t.Fatalf("expected HTTP 400, got %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if got := rebootRequests.Load(); got != 0 {
		t.Fatalf("expected no reboot request, got %d", got)
	}
}

func testTool(server *httptest.Server) config.Tool {
	return config.Tool{
		InterlockName: "test-tool",
		IP:            server.Listener.Addr().String(),
		SwitchID:      0,
		Enabled:       true,
	}
}

func authenticatedTestTool(server *httptest.Server) config.Tool {
	username := "admin"
	password := "password"
	tool := testTool(server)
	tool.Username = &username
	tool.Password = &password
	return tool
}

func TestDigestNonceIsReusedAndNonceCountIncrements(t *testing.T) {
	var challengeRequests atomic.Int32
	var mu sync.Mutex
	var nonceCounts []int

	ncPattern := regexp.MustCompile(`\bnc=([0-9a-fA-F]{8})\b`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		if authorization == "" {
			challengeRequests.Add(1)
			w.Header().Set(
				"WWW-Authenticate",
				`Digest realm="shelly", nonce="nonce-one", algorithm=SHA-256, qop="auth"`,
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		match := ncPattern.FindStringSubmatch(authorization)
		if len(match) != 2 {
			t.Errorf("Authorization header missing nonce count: %q", authorization)
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}

		parsed, err := strconv.ParseInt(match[1], 16, 32)
		if err != nil {
			t.Errorf("parse nonce count: %v", err)
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}

		mu.Lock()
		nonceCounts = append(nonceCounts, int(parsed))
		mu.Unlock()

		fmt.Fprint(w, `{"id":0,"output":false}`)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := authenticatedTestTool(server)

	for i := 0; i < 2; i++ {
		if _, err := client.GetStatus(context.Background(), tool); err != nil {
			t.Fatal(err)
		}
	}

	if got := challengeRequests.Load(); got != 1 {
		t.Fatalf("challenge requests = %d, want 1", got)
	}

	mu.Lock()
	defer mu.Unlock()

	if fmt.Sprint(nonceCounts) != "[1 2]" {
		t.Fatalf("nonce counts = %v, want [1 2]", nonceCounts)
	}
}

func TestConcurrentAuthenticatedRequestsUseNonceCountsInOrder(t *testing.T) {
	var mu sync.Mutex
	var nonceCounts []string
	ncPattern := regexp.MustCompile(`\bnc=([0-9a-fA-F]{8})\b`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		if authorization == "" {
			w.Header().Set(
				"WWW-Authenticate",
				`Digest realm="shelly", nonce="nonce-one", algorithm=SHA-256, qop="auth"`,
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		match := ncPattern.FindStringSubmatch(authorization)
		if len(match) != 2 {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}

		mu.Lock()
		nonceCounts = append(nonceCounts, match[1])
		mu.Unlock()

		time.Sleep(10 * time.Millisecond)
		fmt.Fprint(w, `{"id":0,"output":false}`)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	tool := authenticatedTestTool(server)

	var wg sync.WaitGroup
	errs := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.GetStatus(context.Background(), tool)
			errs <- err
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if strings.Join(nonceCounts, ",") != "00000001,00000002" {
		t.Fatalf("nonce counts = %v, want ordered counts", nonceCounts)
	}
}

func TestTooManyRequestsWaitsRetriesThenSucceeds(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := requests.Add(1)

		switch requestNumber {
		case 1:
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		case 2:
			w.Header().Set(
				"WWW-Authenticate",
				`Digest realm="shelly", nonce="nonce-one", algorithm=SHA-256, qop="auth"`,
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		default:
			fmt.Fprint(w, `{"id":0,"output":true}`)
		}
	}))
	defer server.Close()

	client := NewClient(time.Second)
	client.authenticationThrottleDelay = 5 * time.Millisecond

	status, err := client.GetStatus(
		context.Background(),
		authenticatedTestTool(server),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Output {
		t.Fatal("status output = false, want true")
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("requests = %d, want 3", got)
	}
}

func TestPersistentTooManyRequestsSchedulesAuthenticatedReboot(t *testing.T) {
	var statusRequests atomic.Int32
	rebootSeen := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/Switch.GetStatus":
			statusRequests.Add(1)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)

		case "/rpc/Shelly.Reboot":
			if r.Header.Get("Authorization") == "" {
				w.Header().Set(
					"WWW-Authenticate",
					`Digest realm="shelly", nonce="reboot-nonce", algorithm=SHA-256, qop="auth"`,
				)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			rebootSeen <- struct{}{}
			fmt.Fprint(w, `null`)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(time.Second)
	client.authenticationThrottleDelay = 5 * time.Millisecond
	client.rebootCooldown = time.Hour
	tool := authenticatedTestTool(server)

	_, err := client.GetStatus(context.Background(), tool)
	if !IsHTTPStatus(err, http.StatusTooManyRequests) {
		t.Fatalf("expected HTTP 429, got %v", err)
	}

	if got := statusRequests.Load(); got != 2 {
		t.Fatalf("status requests = %d, want 2", got)
	}

	select {
	case <-rebootSeen:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for authenticated reboot request")
	}
}
