package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
	"github.com/williamveith/fbs-interlock-gateway/internal/shelly"
)

type fakeConfigStore struct {
	cfg         config.Config
	safeOutput  bool
	updateErr   error
	updated     config.Config
	updateCalls int
}

func (f *fakeConfigStore) ConfigSnapshot() config.Config {
	return f.cfg
}

func (f *fakeConfigStore) UpdateConfig(newCfg config.Config) error {
	f.updateCalls++
	f.updated = newCfg

	if f.updateErr != nil {
		return f.updateErr
	}

	f.cfg = newCfg
	return nil
}

func (f *fakeConfigStore) SafeOutput() bool {
	return f.safeOutput
}

type fakeStatusClient struct {
	getStatus func(
		ctx context.Context,
		tool config.Tool,
	) (shelly.SwitchStatus, error)
}

func (f fakeStatusClient) GetStatus(
	ctx context.Context,
	tool config.Tool,
) (shelly.SwitchStatus, error) {
	return f.getStatus(ctx, tool)
}

func TestHandleConfigGet(t *testing.T) {
	stored := config.Config{
		Bind: "127.0.0.1",
		Defaults: config.Defaults{
			TimeoutMS:        800,
			SafeStateOnError: "off",
			ShellyTLS: config.ShellyTLSConfig{
				ServerCAFile:   "/etc/fbs-interlock-gateway/tls/server-ca.crt",
				ClientCertFile: "/etc/fbs-interlock-gateway/tls/gateway-client.crt",
				ClientKeyFile:  "/etc/fbs-interlock-gateway/tls/gateway-client.key",
			},
		},
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-01",
				IP:            "192.0.2.10",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
			{
				InterlockName: "EQU-TEST-02",
				IP:            "192.0.2.11",
				Protocol:      "https",
				Port:          8082,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	expected := stored
	expected.Tools = append([]config.Tool(nil), stored.Tools...)
	expected.Tools[0].Protocol = "http"

	store := &fakeConfigStore{cfg: stored}

	server := New(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		nil,
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/config",
		nil,
	)

	response := httptest.NewRecorder()

	server.handleConfig(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusOK,
			response.Code,
			response.Body.String(),
		)
	}

	if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf(
			"expected application/json content type, got %q",
			contentType,
		)
	}

	var actual config.Config

	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"unexpected config\nexpected: %#v\nactual:   %#v",
			expected,
			actual,
		)
	}
}

func TestHandleConfigPut(t *testing.T) {
	requestConfig := config.Config{
		Bind: "0.0.0.0",
		Defaults: config.Defaults{
			TimeoutMS:        1200,
			SafeStateOnError: "off",
			ShellyTLS: config.ShellyTLSConfig{
				ServerCAFile:   "/etc/fbs-interlock-gateway/tls/server-ca.crt",
				ClientCertFile: "/etc/fbs-interlock-gateway/tls/gateway-client.crt",
				ClientKeyFile:  "/etc/fbs-interlock-gateway/tls/gateway-client.key",
			},
		},
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-02",
				IP:            "192.0.2.20",
				Port:          8082,
				SwitchID:      0,
				Enabled:       true,
			},
			{
				InterlockName: "EQU-TEST-03",
				IP:            "192.0.2.21",
				Protocol:      " HTTPS ",
				Port:          8083,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	expected := requestConfig
	expected.Tools = append([]config.Tool(nil), requestConfig.Tools...)
	expected.Tools[0].Protocol = "http"
	expected.Tools[1].Protocol = "https"

	body, err := json.Marshal(requestConfig)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}

	store := &fakeConfigStore{}

	server := New(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		nil,
	)

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/config",
		bytes.NewReader(body),
	)

	response := httptest.NewRecorder()

	server.handleConfig(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusOK,
			response.Code,
			response.Body.String(),
		)
	}

	if store.updateCalls != 1 {
		t.Fatalf(
			"expected UpdateConfig to be called once, got %d",
			store.updateCalls,
		)
	}

	if !reflect.DeepEqual(store.updated, expected) {
		t.Fatalf(
			"unexpected updated config\nexpected: %#v\nactual:   %#v",
			expected,
			store.updated,
		)
	}

	var result map[string]bool

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !result["saved"] {
		t.Fatal("expected saved=true")
	}

	if !result["restart_required"] {
		t.Fatal("expected restart_required=true")
	}
}

func TestHandleConfigPutRejectsInvalidJSON(t *testing.T) {
	store := &fakeConfigStore{}

	server := New(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		nil,
	)

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/config",
		strings.NewReader(`{"bind":`),
	)

	response := httptest.NewRecorder()

	server.handleConfig(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			response.Code,
		)
	}

	if store.updateCalls != 0 {
		t.Fatalf(
			"expected UpdateConfig not to be called, got %d calls",
			store.updateCalls,
		)
	}

	if !strings.Contains(response.Body.String(), "invalid JSON") {
		t.Fatalf(
			"expected invalid JSON error, got %q",
			response.Body.String(),
		)
	}
}

func TestHandleConfigPutReturnsStoreError(t *testing.T) {
	store := &fakeConfigStore{
		updateErr: errors.New("configuration rejected"),
	}

	server := New(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		nil,
	)

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/config",
		strings.NewReader(`{"bind":"0.0.0.0","tools":[]}`),
	)

	response := httptest.NewRecorder()

	server.handleConfig(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			response.Code,
		)
	}

	if store.updateCalls != 1 {
		t.Fatalf(
			"expected UpdateConfig to be called once, got %d",
			store.updateCalls,
		)
	}

	if !strings.Contains(
		response.Body.String(),
		"configuration rejected",
	) {
		t.Fatalf(
			"expected store error in response, got %q",
			response.Body.String(),
		)
	}
}

func TestHandleConfigRejectsUnsupportedMethod(t *testing.T) {
	store := &fakeConfigStore{}

	server := New(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		nil,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/config",
		nil,
	)

	response := httptest.NewRecorder()

	server.handleConfig(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusMethodNotAllowed,
			response.Code,
		)
	}

	if store.updateCalls != 0 {
		t.Fatalf(
			"expected no config update, got %d calls",
			store.updateCalls,
		)
	}
}

func TestCollectStatuses(t *testing.T) {
	store := &fakeConfigStore{
		safeOutput: true,
		cfg: config.Config{
			Tools: []config.Tool{
				{
					InterlockName: "DISABLED",
					IP:            "192.0.2.10",
					Port:          8081,
					SwitchID:      0,
					Enabled:       false,
				},
				{
					InterlockName: "CONNECTED",
					IP:            "192.0.2.11",
					Protocol:      "https",
					Port:          8082,
					SwitchID:      0,
					Enabled:       true,
				},
				{
					InterlockName: "FAILED",
					IP:            "192.0.2.12",
					Port:          8083,
					SwitchID:      0,
					Enabled:       true,
				},
			},
		},
	}

	statusClient := fakeStatusClient{
		getStatus: func(
			_ context.Context,
			tool config.Tool,
		) (shelly.SwitchStatus, error) {
			switch tool.InterlockName {
			case "CONNECTED":
				return shelly.SwitchStatus{
					ID:     tool.SwitchID,
					Output: true,
				}, nil

			case "FAILED":
				return shelly.SwitchStatus{},
					errors.New("Shelly unreachable")

			default:
				return shelly.SwitchStatus{},
					errors.New("unexpected status request")
			}
		},
	}

	server := New(
		"127.0.0.1:0",
		store,
		statusClient,
		nil,
	)

	results, err := server.collectStatuses(context.Background())
	if err != nil {
		t.Fatalf("collectStatuses() error = %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	disabled := results[0]

	if disabled.InterlockName != "DISABLED" {
		t.Fatalf(
			"expected first result to be DISABLED, got %q",
			disabled.InterlockName,
		)
	}

	if disabled.Enabled {
		t.Fatal("expected disabled tool to have enabled=false")
	}

	if disabled.Connected {
		t.Fatal("expected disabled tool to have connected=false")
	}

	if disabled.Output {
		t.Fatal("expected disabled tool output to remain false")
	}

	if disabled.Error != "" {
		t.Fatalf(
			"expected disabled tool error to be empty, got %q",
			disabled.Error,
		)
	}

	if disabled.Protocol != "http" {
		t.Fatalf("disabled protocol = %q, want http", disabled.Protocol)
	}

	connected := results[1]

	if !connected.Enabled {
		t.Fatal("expected connected tool to have enabled=true")
	}

	if !connected.Connected {
		t.Fatal("expected connected tool to have connected=true")
	}

	if !connected.Output {
		t.Fatal("expected connected tool output=true")
	}

	if connected.Error != "" {
		t.Fatalf(
			"expected connected tool error to be empty, got %q",
			connected.Error,
		)
	}

	if connected.Protocol != "https" {
		t.Fatalf("connected protocol = %q, want https", connected.Protocol)
	}

	failed := results[2]

	if !failed.Enabled {
		t.Fatal("expected failed tool to have enabled=true")
	}

	if failed.Connected {
		t.Fatal("expected failed tool to have connected=false")
	}

	if !failed.Output {
		t.Fatal(
			"expected failed tool output to use safe output=true",
		)
	}

	if failed.Error != "Shelly unreachable" {
		t.Fatalf(
			"expected Shelly error, got %q",
			failed.Error,
		)
	}

	if failed.Protocol != "http" {
		t.Fatalf("failed protocol = %q, want http", failed.Protocol)
	}
}

func TestHandleRestart(t *testing.T) {
	restartRequested := make(chan struct{}, 1)

	server := New(
		"127.0.0.1:0",
		&fakeConfigStore{},
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		restartRequested,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/restart",
		nil,
	)

	response := httptest.NewRecorder()

	server.handleRestart(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusOK,
			response.Code,
			response.Body.String(),
		)
	}

	var result map[string]bool

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode restart response: %v", err)
	}

	if !result["restart_requested"] {
		t.Fatal("expected restart_requested=true")
	}

	select {
	case <-restartRequested:
		// Expected.
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for restart request")
	}
}

func TestHandleRestartRejectsUnsupportedMethod(t *testing.T) {
	restartRequested := make(chan struct{}, 1)

	server := New(
		"127.0.0.1:0",
		&fakeConfigStore{},
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				return shelly.SwitchStatus{}, nil
			},
		},
		restartRequested,
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/restart",
		nil,
	)

	response := httptest.NewRecorder()

	server.handleRestart(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusMethodNotAllowed,
			response.Code,
		)
	}

	select {
	case <-restartRequested:
		t.Fatal(
			"restart signal should not be sent for unsupported method",
		)
	default:
		// Expected.
	}
}

func TestConcurrentStatusRequestsShareOneRefresh(t *testing.T) {
	store := &fakeConfigStore{cfg: config.Config{Tools: []config.Tool{{
		InterlockName: "TEST",
		IP:            "192.0.2.10",
		Port:          8081,
		SwitchID:      0,
		Enabled:       true,
	}}}}

	var calls atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := New(
		"127.0.0.1:0",
		store,
		fakeStatusClient{getStatus: func(ctx context.Context, tool config.Tool) (shelly.SwitchStatus, error) {
			calls.Add(1)
			started <- struct{}{}
			select {
			case <-release:
				return shelly.SwitchStatus{Output: true}, nil
			case <-ctx.Done():
				return shelly.SwitchStatus{}, ctx.Err()
			}
		}},
		nil,
	)

	first := httptest.NewRecorder()
	second := httptest.NewRecorder()
	done := make(chan struct{}, 2)

	go func() {
		server.handleStatus(first, httptest.NewRequest(http.MethodGet, "/api/status", nil))
		done <- struct{}{}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first status refresh did not start")
	}

	go func() {
		server.handleStatus(second, httptest.NewRequest(http.MethodGet, "/api/status", nil))
		done <- struct{}{}
	}()

	time.Sleep(25 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("GetStatus calls while refresh in flight = %d, want 1", got)
	}

	close(release)
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("status request did not finish")
		}
	}

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("response codes = %d and %d, want 200 and 200", first.Code, second.Code)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("GetStatus calls = %d, want 1", got)
	}
}

func TestStatusSnapshotUsesShortCache(t *testing.T) {
	store := &fakeConfigStore{
		cfg: config.Config{
			Tools: []config.Tool{
				{
					InterlockName: "TEST",
					IP:            "192.0.2.10",
					Port:          8081,
					SwitchID:      0,
					Enabled:       true,
				},
			},
		},
	}

	var calls atomic.Int32

	server := New(
		"127.0.0.1:0",
		store,
		fakeStatusClient{
			getStatus: func(
				context.Context,
				config.Tool,
			) (shelly.SwitchStatus, error) {
				calls.Add(1)

				return shelly.SwitchStatus{
					Output: true,
				}, nil
			},
		},
		nil,
	)

	// The first request returns immediately and starts the refresh in the
	// background.
	first := httptest.NewRecorder()

	server.handleStatus(
		first,
		httptest.NewRequest(
			http.MethodGet,
			"/api/status",
			nil,
		),
	)

	if first.Code != http.StatusOK {
		t.Fatalf(
			"first response code = %d, want 200",
			first.Code,
		)
	}

	// Wait until the asynchronous refresh has completed and populated the
	// cache. Read the state under the same mutex used by the server.
	deadline := time.Now().Add(time.Second)

	for {
		server.statusMu.Lock()

		refreshComplete :=
			!server.statusRefreshInFlight &&
				!server.statusCacheAt.IsZero()

		server.statusMu.Unlock()

		if refreshComplete {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal(
				"status refresh did not complete within one second",
			)
		}

		time.Sleep(time.Millisecond)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"GetStatus calls after initial refresh = %d, want 1",
			got,
		)
	}

	// This request occurs within statusCacheTTL and must use the completed
	// cache rather than starting another device request.
	second := httptest.NewRecorder()

	server.handleStatus(
		second,
		httptest.NewRequest(
			http.MethodGet,
			"/api/status",
			nil,
		),
	)

	if second.Code != http.StatusOK {
		t.Fatalf(
			"second response code = %d, want 200",
			second.Code,
		)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"GetStatus calls after cached request = %d, want 1",
			got,
		)
	}
}
