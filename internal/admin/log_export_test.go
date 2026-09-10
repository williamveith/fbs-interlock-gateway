package admin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testLogExporter struct {
	filename string
	body     string
	err      error
	calls    int
}

func (e *testLogExporter) Export(_ context.Context, dst io.Writer) error {
	e.calls++
	if e.err != nil {
		return e.err
	}
	_, err := io.WriteString(dst, e.body)
	return err
}

func (e *testLogExporter) ExportFilename(time.Time) string {
	return e.filename
}

func TestHandleLogExportRejectsWrongMethod(t *testing.T) {
	exporter := &testLogExporter{}
	server := &Server{logExporter: exporter}

	request := httptest.NewRequest(http.MethodGet, "/api/logs/export", nil)
	response := httptest.NewRecorder()

	server.handleLogExport(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if exporter.calls != 0 {
		t.Fatalf("export calls = %d, want 0", exporter.calls)
	}
}

func TestHandleLogExportUnavailable(t *testing.T) {
	server := &Server{}

	request := httptest.NewRequest(http.MethodPost, "/api/logs/export", nil)
	response := httptest.NewRecorder()

	server.handleLogExport(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleLogExportStreamsAttachment(t *testing.T) {
	exporter := &testLogExporter{
		filename: "logs-test.log",
		body:     "one\ntwo\n",
	}
	server := &Server{logExporter: exporter}

	request := httptest.NewRequest(http.MethodPost, "/api/logs/export", nil)
	response := httptest.NewRecorder()

	server.handleLogExport(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if exporter.calls != 1 {
		t.Fatalf("export calls = %d, want 1", exporter.calls)
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header().Get("Content-Disposition"); !strings.Contains(got, "logs-test.log") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if response.Body.String() != exporter.body {
		t.Fatalf("body = %q, want %q", response.Body.String(), exporter.body)
	}
}

func TestHandleLogExportFailureDoesNotPanic(t *testing.T) {
	exporter := &testLogExporter{
		filename: "logs-test.log",
		err:      errors.New("export failed"),
	}
	server := &Server{logExporter: exporter}

	request := httptest.NewRequest(http.MethodPost, "/api/logs/export", nil)
	response := httptest.NewRecorder()

	server.handleLogExport(response, request)

	if exporter.calls != 1 {
		t.Fatalf("export calls = %d, want 1", exporter.calls)
	}
}
