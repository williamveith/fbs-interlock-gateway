package fbs

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
	"github.com/williamveith/fbs-interlock-gateway/internal/shelly"
)

type fakeShellyClient struct {
	status    shelly.SwitchStatus
	statusErr error
	setErr    error
	setCalls  []bool
}

func (f *fakeShellyClient) GetStatus(context.Context, config.Tool) (shelly.SwitchStatus, error) {
	return f.status, f.statusErr
}

func (f *fakeShellyClient) Set(_ context.Context, _ config.Tool, on bool) error {
	f.setCalls = append(f.setCalls, on)
	return f.setErr
}

func testTool() config.Tool {
	return config.Tool{
		InterlockName: "EQU-TEST-01",
		IP:            "127.0.0.1",
		Port:          8081,
		SwitchID:      0,
		Enabled:       true,
	}
}

func TestStatusReturnsShellyOutput(t *testing.T) {
	client := &fakeShellyClient{status: shelly.SwitchStatus{ID: 0, Output: true}}
	server := NewServer("127.0.0.1", false, client)

	req := httptest.NewRequest("GET", "http://gateway/status", nil)
	res := httptest.NewRecorder()
	server.handleFBSRequest(res, req, testTool())

	if res.Code != 200 {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if got := res.Body.String(); got != `{"Success":1,"State":1}` {
		t.Fatalf("body = %q", got)
	}
}

func TestOnAndOffCommands(t *testing.T) {
	tests := []struct {
		path string
		want bool
		body string
	}{
		{path: "/on", want: true, body: `{"Success":1,"State":1}`},
		{path: "/off", want: false, body: `{"Success":1,"State":0}`},
		{path: "/?state=1", want: true, body: `{"Success":1,"State":1}`},
		{path: "/?state=0", want: false, body: `{"Success":1,"State":0}`},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			client := &fakeShellyClient{}
			server := NewServer("127.0.0.1", false, client)

			req := httptest.NewRequest("GET", "http://gateway"+tt.path, nil)
			res := httptest.NewRecorder()
			server.handleFBSRequest(res, req, testTool())

			if len(client.setCalls) != 1 || client.setCalls[0] != tt.want {
				t.Fatalf("set calls = %v, want [%t]", client.setCalls, tt.want)
			}
			if got := res.Body.String(); got != tt.body {
				t.Fatalf("body = %q, want %q", got, tt.body)
			}
		})
	}
}

func TestShellyFailureReturnsConfiguredSafeState(t *testing.T) {
	client := &fakeShellyClient{statusErr: errors.New("offline")}
	server := NewServer("127.0.0.1", true, client)

	req := httptest.NewRequest("GET", "http://gateway/status", nil)
	res := httptest.NewRecorder()
	server.handleFBSRequest(res, req, testTool())

	if got := res.Body.String(); got != `{"Success":1,"State":1}` {
		t.Fatalf("body = %q, want safe-on response", got)
	}
}

func TestUnknownRequestReturnsSafeStateWithoutChangingRelay(t *testing.T) {
	client := &fakeShellyClient{}
	server := NewServer("127.0.0.1", false, client)

	req := httptest.NewRequest("GET", "http://gateway/not-a-command", nil)
	res := httptest.NewRecorder()
	server.handleFBSRequest(res, req, testTool())

	if len(client.setCalls) != 0 {
		t.Fatalf("unexpected Set calls: %v", client.setCalls)
	}
	if got := res.Body.String(); got != `{"Success":1,"State":0}` {
		t.Fatalf("body = %q, want safe-off response", got)
	}
}
