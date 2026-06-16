package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
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
	cfg    Config
	client *http.Client
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config.yaml")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
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

	g := &Gateway{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.Defaults.TimeoutMS) * time.Millisecond,
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	for _, tool := range cfg.Tools {
		tool := tool

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
		Handler:           recoverMiddleware(loggingMiddleware(mux)),
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

	log.Printf("tool=%s fbs_request method=%s path=%s query=%s remote=%s", tool.InterlockName, r.Method, r.URL.Path, r.URL.RawQuery, r.RemoteAddr)

	switch {
	case strings.Contains(full, "status"):
		g.handleStatus(w, tool)

	case isOnRequest(full):
		g.handleSet(w, tool, true)

	case isOffRequest(full):
		g.handleSet(w, tool, false)

	default:
		// Return a controlled response instead of crashing/erroring hard.
		writeJSON(w, http.StatusNotFound, map[string]any{
			"status": "unknown_request",
			"tool":   tool.InterlockName,
			"path":   r.URL.Path,
			"query":  r.URL.RawQuery,
		})
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

		safeOutput := false
		if strings.EqualFold(g.cfg.Defaults.SafeStateOnError, "on") {
			safeOutput = true
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "error",
			"connected": false,
			"tool":      tool.InterlockName,
			"relay":     onOff(safeOutput),
			"state":     onOff(safeOutput),
			"output":    safeOutput,
			"ison":      safeOutput,
			"error":     err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"connected": true,
		"tool":      tool.InterlockName,
		"relay":     onOff(status.Output),
		"state":     onOff(status.Output),
		"output":    status.Output,
		"ison":      status.Output,
	})
}

func (g *Gateway) handleSet(w http.ResponseWriter, tool Tool, on bool) {
	err := g.setShelly(tool, on)
	if err != nil {
		log.Printf("tool=%s shelly_set_error command=%s error=%v", tool.InterlockName, onOff(on), err)

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "error",
			"connected": false,
			"tool":      tool.InterlockName,
			"command":   onOff(on),
			"error":     err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"connected": true,
		"tool":      tool.InterlockName,
		"command":   onOff(on),
		"relay":     onOff(on),
		"state":     onOff(on),
		"output":    on,
		"ison":      on,
	})
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

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return status, fmt.Errorf("shelly status HTTP %d: %s", resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, &status); err != nil {
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

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("shelly set HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func writeJSON(w http.ResponseWriter, code int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("http_request method=%s path=%s remote=%s duration=%s", r.Method, r.URL.String(), r.RemoteAddr, time.Since(start))
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic recovered: %v", recovered)

				writeJSON(w, http.StatusOK, map[string]any{
					"status":    "error",
					"connected": false,
					"error":     fmt.Sprintf("%v", recovered),
				})
			}
		}()

		next.ServeHTTP(w, r)
	})
}
