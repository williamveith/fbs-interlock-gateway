package fbs

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
	"github.com/williamveith/fbs-interlock-gateway/internal/shelly"
)

type ShellyClient interface {
	GetStatus(ctx context.Context, tool config.Tool) (shelly.SwitchStatus, error)
	Set(ctx context.Context, tool config.Tool, on bool) error
}

type Server struct {
	bind       string
	safeOutput bool
	shelly     ShellyClient
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	body   strings.Builder
}

var (
	fbsOn  = []byte(`{"Success":1,"State":1}`)
	fbsOff = []byte(`{"Success":1,"State":0}`)
)

func NewServer(bind string, safeOutput bool, shellyClient ShellyClient) *Server {
	return &Server{
		bind:       bind,
		safeOutput: safeOutput,
		shelly:     shellyClient,
	}
}

func (s *Server) RunToolServer(ctx context.Context, tool config.Tool) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.handleFBSRequest(w, r, tool)
	})

	addr := net.JoinHostPort(s.bind, fmt.Sprintf("%d", tool.Port))

	server := &http.Server{
		Addr:              addr,
		Handler:           recoverMiddleware(loggingMiddleware(mux), s.safeOutput),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("tool=%s listening on %s -> shelly=%s switch=%d", tool.InterlockName, addr, tool.IP, tool.SwitchID)

	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error for tool=%s port=%d: %w", tool.InterlockName, tool.Port, err)
	}

	return nil
}

func (s *Server) handleFBSRequest(w http.ResponseWriter, r *http.Request, tool config.Tool) {
	path := strings.ToLower(r.URL.Path)
	query := strings.ToLower(r.URL.RawQuery)
	full := path + "?" + query

	logFBSRequest(tool, r)

	switch {
	case strings.Contains(full, "status"):
		s.handleStatus(w, r, tool)

	case isOnRequest(full):
		s.handleSet(w, r, tool, true)

	case isOffRequest(full):
		s.handleSet(w, r, tool, false)

	default:
		log.Printf("tool=%s unknown_request path=%s query=%s", tool.InterlockName, r.URL.Path, r.URL.RawQuery)
		writeFBS(w, false)
	}
}

func isOnRequest(s string) bool {
	return strings.Contains(s, "/on") ||
		strings.Contains(s, "turn=on") ||
		strings.Contains(s, "state=on") ||
		strings.Contains(s, "state=1") ||
		strings.Contains(s, "value=1") ||
		strings.Contains(s, "true")
}

func isOffRequest(s string) bool {
	return strings.Contains(s, "/off") ||
		strings.Contains(s, "turn=off") ||
		strings.Contains(s, "state=off") ||
		strings.Contains(s, "state=0") ||
		strings.Contains(s, "value=0") ||
		strings.Contains(s, "false")
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, tool config.Tool) {
	status, err := s.shelly.GetStatus(r.Context(), tool)
	if err != nil {
		log.Printf("tool=%s shelly_status_error=%v", tool.InterlockName, err)
		writeFBS(w, s.safeOutput)
		return
	}

	writeFBS(w, status.Output)
}

func (s *Server) handleSet(w http.ResponseWriter, r *http.Request, tool config.Tool, on bool) {
	err := s.shelly.Set(r.Context(), tool, on)
	if err != nil {
		log.Printf("tool=%s shelly_set_error command=%s error=%v", tool.InterlockName, onOff(on), err)
		writeFBS(w, s.safeOutput)
		return
	}

	writeFBS(w, on)
}

func writeFBS(w http.ResponseWriter, state bool) {
	body := fbsOff
	if state {
		body = fbsOn
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(body); err != nil {
		log.Printf("failed to write FBS response: %v", err)
	}
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(data []byte) (int, error) {
	rr.body.Write(data)
	return rr.ResponseWriter.Write(data)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rr := &responseRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(rr, r)

		responseBody := rr.body.String()
		if len(responseBody) > 2048 {
			responseBody = responseBody[:2048] + "...[truncated]"
		}

		log.Printf(
			"FBS_OUT method=%s url=%s remote=%s status=%d duration=%s body=%q",
			r.Method,
			r.URL.String(),
			r.RemoteAddr,
			rr.status,
			time.Since(start),
			responseBody,
		)
	})
}

func logFBSRequest(tool config.Tool, r *http.Request) {
	body := ""

	if r.Body != nil && r.ContentLength != 0 {
		data, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
		if err == nil {
			body = string(data)
		}

		r.Body = io.NopCloser(strings.NewReader(body))
	}

	log.Printf(
		"FBS_IN tool=%s method=%s url=%s proto=%s host=%s remote=%s user_agent=%q content_type=%q accept=%q auth_present=%t body=%q",
		tool.InterlockName,
		r.Method,
		r.URL.String(),
		r.Proto,
		r.Host,
		r.RemoteAddr,
		r.UserAgent(),
		r.Header.Get("Content-Type"),
		r.Header.Get("Accept"),
		r.Header.Get("Authorization") != "",
		body,
	)
}

func recoverMiddleware(next http.Handler, safeOutput bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic recovered: %v", recovered)
				writeFBS(w, safeOutput)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
