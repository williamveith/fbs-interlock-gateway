package fbs

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
	"github.com/williamveith/fbs-interlock-gateway/internal/shelly"
)

type fuzzShellyClient struct {
	statusOutput bool
	err          error
	getCalls     int
	setCalls     []bool
}

func (f *fuzzShellyClient) GetStatus(
	context.Context,
	config.Tool,
) (shelly.SwitchStatus, error) {
	f.getCalls++

	if f.err != nil {
		return shelly.SwitchStatus{}, f.err
	}

	return shelly.SwitchStatus{
		ID:     0,
		Output: f.statusOutput,
	}, nil
}

func (f *fuzzShellyClient) Set(
	_ context.Context,
	_ config.Tool,
	on bool,
) error {
	f.setCalls = append(f.setCalls, on)
	return f.err
}

type fuzzStatusRecorder struct {
	nextRevisionCalls int
	successCalls      []fuzzStatusSuccess
	failureCalls      []fuzzStatusFailure
}

type fuzzStatusSuccess struct {
	tool     config.Tool
	output   bool
	revision uint64
}

type fuzzStatusFailure struct {
	tool       config.Tool
	safeOutput bool
	err        error
	revision   uint64
}

func (f *fuzzStatusRecorder) NextRevision() uint64 {
	f.nextRevisionCalls++
	return uint64(f.nextRevisionCalls)
}

func (f *fuzzStatusRecorder) RecordSuccess(
	tool config.Tool,
	output bool,
	revision uint64,
) {
	f.successCalls = append(f.successCalls, fuzzStatusSuccess{
		tool:     tool,
		output:   output,
		revision: revision,
	})
}

func (f *fuzzStatusRecorder) RecordFailure(
	tool config.Tool,
	safeOutput bool,
	err error,
	revision uint64,
) {
	f.failureCalls = append(f.failureCalls, fuzzStatusFailure{
		tool:       tool,
		safeOutput: safeOutput,
		err:        err,
		revision:   revision,
	})
}

func FuzzFBSRequestHandling(f *testing.F) {
	// Valid requests across successful and failed Shelly operations.
	f.Add("GET", "/status", "", false, true, false)
	f.Add("GET", "/status", "", true, false, true)
	f.Add("GET", "/on", "", false, false, false)
	f.Add("GET", "/on", "", true, true, true)
	f.Add("GET", "/off", "", false, true, false)
	f.Add("GET", "/off", "", true, false, true)

	// Invalid routing and request-shape cases.
	f.Add("POST", "/on", "", false, false, false)
	f.Add("GET", "/on/", "", false, false, false)
	f.Add("GET", "//on", "", false, false, false)
	f.Add("GET", "/ON", "", false, false, false)
	f.Add("GET", "/status", "refresh=1", false, false, false)
	f.Add("", "", "", false, false, false)

	// Fuzzing this handler can otherwise spend most of its time formatting
	// expected rejection logs rather than exploring inputs.
	previousLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	f.Cleanup(func() {
		log.SetOutput(previousLogOutput)
	})

	tool := config.Tool{
		InterlockName: "FUZZ-TOOL",
		IP:            "127.0.0.1",
		Port:          8081,
		SwitchID:      0,
		Enabled:       true,
	}

	f.Fuzz(func(
		t *testing.T,
		method string,
		path string,
		query string,
		safeOutput bool,
		statusOutput bool,
		operationFails bool,
	) {
		if len(method) > 64 || len(path) > 4096 || len(query) > 4096 {
			t.Skip()
		}

		var operationErr error
		if operationFails {
			operationErr = errors.New("fuzz Shelly operation failed")
		}

		client := &fuzzShellyClient{
			statusOutput: statusOutput,
			err:          operationErr,
		}
		recorder := &fuzzStatusRecorder{}

		server := NewServer(
			"127.0.0.1",
			safeOutput,
			client,
			recorder,
		)

		req := &http.Request{
			Method: method,
			URL: &url.URL{
				Path:     path,
				RawQuery: query,
			},
			Header: make(http.Header),
		}

		res := httptest.NewRecorder()
		server.handleFBSRequest(res, req, tool)

		validRequest :=
			method == http.MethodGet &&
				query == "" &&
				(path == "/status" || path == "/on" || path == "/off")

		if !validRequest {
			assertRejectedFBSRequest(
				t,
				res,
				client,
				recorder,
				method,
				path,
				query,
			)
			return
		}

		if res.Code != http.StatusOK {
			t.Fatalf(
				"valid request returned status %d: method=%q path=%q",
				res.Code,
				method,
				path,
			)
		}

		if got := res.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", got)
		}

		operationOutput := statusOutput
		switch path {
		case "/status":
			if client.getCalls != 1 {
				t.Fatalf("GetStatus calls = %d, want 1", client.getCalls)
			}
			if len(client.setCalls) != 0 {
				t.Fatalf("status unexpectedly called Set: %v", client.setCalls)
			}

		case "/on":
			operationOutput = true
			if client.getCalls != 0 {
				t.Fatalf("on unexpectedly called GetStatus")
			}
			if len(client.setCalls) != 1 || !client.setCalls[0] {
				t.Fatalf("on Set calls = %v, want [true]", client.setCalls)
			}

		case "/off":
			operationOutput = false
			if client.getCalls != 0 {
				t.Fatalf("off unexpectedly called GetStatus")
			}
			if len(client.setCalls) != 1 || client.setCalls[0] {
				t.Fatalf("off Set calls = %v, want [false]", client.setCalls)
			}
		}

		responseOutput := operationOutput
		if operationFails {
			responseOutput = safeOutput
		}

		wantBody := `{"Success":1,"State":0}`
		if responseOutput {
			wantBody = `{"Success":1,"State":1}`
		}

		if got := res.Body.String(); got != wantBody {
			t.Fatalf("body = %q, want %q", got, wantBody)
		}

		if got := res.Header().Get("Content-Length"); got != strconv.Itoa(len(wantBody)) {
			t.Fatalf("Content-Length = %q, want %d", got, len(wantBody))
		}

		if recorder.nextRevisionCalls != 1 {
			t.Fatalf(
				"NextRevision calls = %d, want 1",
				recorder.nextRevisionCalls,
			)
		}

		if operationFails {
			if len(recorder.successCalls) != 0 {
				t.Fatalf("failed operation recorded success: %#v", recorder.successCalls)
			}
			if len(recorder.failureCalls) != 1 {
				t.Fatalf("failure calls = %d, want 1", len(recorder.failureCalls))
			}

			failure := recorder.failureCalls[0]
			if failure.tool.InterlockName != tool.InterlockName ||
				failure.tool.Port != tool.Port {
				t.Fatalf("failure recorded wrong tool: %#v", failure.tool)
			}
			if failure.safeOutput != safeOutput {
				t.Fatalf(
					"recorded safeOutput = %t, want %t",
					failure.safeOutput,
					safeOutput,
				)
			}
			if failure.err != operationErr {
				t.Fatalf("recorded error = %v, want %v", failure.err, operationErr)
			}
			if failure.revision != 1 {
				t.Fatalf("failure revision = %d, want 1", failure.revision)
			}
			return
		}

		if len(recorder.failureCalls) != 0 {
			t.Fatalf("successful operation recorded failure: %#v", recorder.failureCalls)
		}
		if len(recorder.successCalls) != 1 {
			t.Fatalf("success calls = %d, want 1", len(recorder.successCalls))
		}

		success := recorder.successCalls[0]
		if success.tool.InterlockName != tool.InterlockName || success.tool.Port != tool.Port {
			t.Fatalf("success recorded wrong tool: %#v", success.tool)
		}
		if success.output != operationOutput {
			t.Fatalf("recorded output = %t, want %t", success.output, operationOutput)
		}
		if success.revision != 1 {
			t.Fatalf("success revision = %d, want 1", success.revision)
		}
	})
}

func assertRejectedFBSRequest(
	t *testing.T,
	res *httptest.ResponseRecorder,
	client *fuzzShellyClient,
	recorder *fuzzStatusRecorder,
	method string,
	path string,
	query string,
) {
	t.Helper()

	if client.getCalls != 0 {
		t.Fatalf(
			"invalid request reached GetStatus: method=%q path=%q query=%q",
			method,
			path,
			query,
		)
	}

	if len(client.setCalls) != 0 {
		t.Fatalf(
			"invalid request reached Set: method=%q path=%q query=%q calls=%v",
			method,
			path,
			query,
			client.setCalls,
		)
	}

	if recorder.nextRevisionCalls != 0 ||
		len(recorder.successCalls) != 0 ||
		len(recorder.failureCalls) != 0 {
		t.Fatalf(
			"invalid request mutated shared status: revisions=%d success=%d failure=%d",
			recorder.nextRevisionCalls,
			len(recorder.successCalls),
			len(recorder.failureCalls),
		)
	}

	wantStatus := http.StatusNotFound
	if method != http.MethodGet {
		wantStatus = http.StatusMethodNotAllowed
	} else if query != "" {
		wantStatus = http.StatusBadRequest
	}

	if res.Code != wantStatus {
		t.Fatalf(
			"rejected request status = %d, want %d: method=%q path=%q query=%q",
			res.Code,
			wantStatus,
			method,
			path,
			query,
		)
	}
}
