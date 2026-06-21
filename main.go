package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Bind     string   `yaml:"bind"`
	Defaults Defaults `yaml:"defaults"`
	Tools    []Tool   `yaml:"tools"`
}

type Defaults struct {
	TimeoutMS        int    `yaml:"timeout_ms"`
	SafeStateOnError string `yaml:"safe_state_on_error"`
}

type Tool struct {
	InterlockName string  `yaml:"interlock_name"`
	IP            string  `yaml:"ip"`
	Port          int     `yaml:"port"`
	SwitchID      int     `yaml:"switch_id"`
	Username      *string `yaml:"username"`
	Password      *string `yaml:"password"`
	Enabled       bool    `yaml:"enabled"`
}

type ShellySwitchStatus struct {
	ID     int  `json:"id"`
	Output bool `json:"output"`
}

type Gateway struct {
	cfg        Config
	client     *http.Client
	safeOutput bool
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	body   strings.Builder
}

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var (
	fbsOn  = []byte(`{"Success":1,"State":1}`)
	fbsOff = []byte(`{"Success":1,"State":0}`)
)

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

func main() {
	// Get the absolute path of the executing binary
	exePath, err := os.Executable()
	if err != nil {
		panic(err)
	}

	// Get the directory name of that path
	dir := filepath.Dir(exePath)

	// Securely stitch the directory and filename together
	filePath := filepath.Join(dir, "config.yaml")

	cfg, err := loadConfig(filePath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if cfg.Bind == "" {
		cfg.Bind = "0.0.0.0"
	}

	if cfg.Defaults.TimeoutMS <= 0 {
		cfg.Defaults.TimeoutMS = 800
	}

	if cfg.Defaults.SafeStateOnError == "" {
		cfg.Defaults.SafeStateOnError = "off"
	}

	log.Printf("fbs-interlock-gateway version=%s commit=%s date=%s", version, commit, date)

	g := &Gateway{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.Defaults.TimeoutMS) * time.Millisecond,
			Transport: &http.Transport{
				MaxIdleConns:        256,
				MaxIdleConnsPerHost: 8,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		safeOutput: strings.EqualFold(cfg.Defaults.SafeStateOnError, "on"),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	for _, tool := range cfg.Tools {
		if !tool.Enabled {
			log.Printf("tool=%s port=%d disabled; skipping", tool.InterlockName, tool.Port)
			continue
		}

		if err := validateTool(tool); err != nil {
			log.Fatalf("invalid config for tool %q: %v", tool.InterlockName, err)
		}

		wg.Add(1)

		go func() {
			defer wg.Done()
			g.runToolServer(ctx, tool)
		}()
	}

	log.Printf("fbs-interlock-gateway started with %d tool(s)", len(cfg.Tools))

	<-ctx.Done()
	log.Println("shutdown requested")
	wg.Wait()
	log.Println("shutdown complete")
}

func loadConfig(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func validateTool(t Tool) error {
	if t.InterlockName == "" {
		return fmt.Errorf("missing interlock_name")
	}
	if t.Port <= 0 || t.Port > 65535 {
		return fmt.Errorf("invalid port %d", t.Port)
	}
	if t.IP == "" {
		return fmt.Errorf("missing ip")
	}
	if t.SwitchID < 0 {
		return fmt.Errorf("invalid switch_id %d", t.SwitchID)
	}
	return nil
}

func (g *Gateway) runToolServer(ctx context.Context, tool Tool) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		g.handleFBSRequest(w, r, tool)
	})

	addr := net.JoinHostPort(g.cfg.Bind, fmt.Sprintf("%d", tool.Port))

	server := &http.Server{
		Addr:              addr,
		Handler:           recoverMiddleware(loggingMiddleware(mux), g.safeOutput),
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
		log.Printf("server error for tool=%s port=%d: %v", tool.InterlockName, tool.Port, err)
	}
}

func (g *Gateway) handleFBSRequest(w http.ResponseWriter, r *http.Request, tool Tool) {
	path := strings.ToLower(r.URL.Path)
	query := strings.ToLower(r.URL.RawQuery)
	full := path + "?" + query

	logFBSRequest(tool, r)

	switch {
	case strings.Contains(full, "status"):
		g.handleStatus(w, tool)

	case isOnRequest(full):
		g.handleSet(w, tool, true)

	case isOffRequest(full):
		g.handleSet(w, tool, false)

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

func (g *Gateway) handleStatus(w http.ResponseWriter, tool Tool) {
	status, err := g.getShellyStatus(tool)
	if err != nil {
		log.Printf("tool=%s shelly_status_error=%v", tool.InterlockName, err)
		writeFBS(w, g.safeOutput)
		return
	}

	writeFBS(w, status.Output)
}

func (g *Gateway) handleSet(w http.ResponseWriter, tool Tool, on bool) {
	err := g.setShelly(tool, on)
	if err != nil {
		log.Printf("tool=%s shelly_set_error command=%s error=%v", tool.InterlockName, onOff(on), err)
		writeFBS(w, g.safeOutput)
		return
	}

	writeFBS(w, on)
}

func (g *Gateway) getShellyStatus(tool Tool) (ShellySwitchStatus, error) {
	var status ShellySwitchStatus

	url := fmt.Sprintf(
		"http://%s/rpc/Switch.GetStatus?id=%d",
		tool.IP,
		tool.SwitchID,
	)

	resp, err := g.client.Get(url)
	if err != nil {
		return status, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return status, fmt.Errorf("shelly status HTTP %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&status); err != nil {
		return status, err
	}

	return status, nil
}

func (g *Gateway) setShelly(tool Tool, on bool) error {
	url := fmt.Sprintf(
		"http://%s/rpc/Switch.Set?id=%d&on=%t",
		tool.IP,
		tool.SwitchID,
		on,
	)

	resp, err := g.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("shelly set HTTP %d: %s", resp.StatusCode, string(body))
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
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

func logFBSRequest(tool Tool, r *http.Request) {
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
