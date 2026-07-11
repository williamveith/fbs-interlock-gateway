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
	expected := config.Config{
		Bind: "127.0.0.1",
		Defaults: config.Defaults{
			TimeoutMS:        800,
			SafeStateOnError: "off",
		},
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-01",
				IP:            "192.0.2.10",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	store := &fakeConfigStore{
		cfg: expected,
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

	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
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
	newConfig := config.Config{
		Bind: "0.0.0.0",
		Defaults: config.Defaults{
			TimeoutMS:        1200,
			SafeStateOnError: "off",
		},
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-02",
				IP:            "192.0.2.20",
				Port:          8082,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	body, err := json.Marshal(newConfig)
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

	if !reflect.DeepEqual(store.updated, newConfig) {
		t.Fatalf(
			"unexpected updated config\nexpected: %#v\nactual:   %#v",
			newConfig,
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

func TestHandleStatus(t *testing.T) {
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

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/status",
		nil,
	)

	response := httptest.NewRecorder()

	server.handleStatus(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusOK,
			response.Code,
			response.Body.String(),
		)
	}

	var results []ToolStatus

	if err := json.NewDecoder(response.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode status response: %v", err)
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
