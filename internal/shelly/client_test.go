package shelly

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
