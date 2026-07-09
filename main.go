package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Bind     string   `yaml:"bind" json:"bind"`
	Defaults Defaults `yaml:"defaults" json:"defaults"`
	Tools    []Tool   `yaml:"tools" json:"tools"`
}

type Defaults struct {
	TimeoutMS        int    `yaml:"timeout_ms" json:"timeout_ms"`
	SafeStateOnError string `yaml:"safe_state_on_error" json:"safe_state_on_error"`
}

type Tool struct {
	InterlockName string  `yaml:"interlock_name" json:"interlock_name"`
	IP            string  `yaml:"ip" json:"ip"`
	Port          int     `yaml:"port" json:"port"`
	SwitchID      int     `yaml:"switch_id" json:"switch_id"`
	Username      *string `yaml:"username" json:"username"`
	Password      *string `yaml:"password" json:"password"`
	Enabled       bool    `yaml:"enabled" json:"enabled"`
}

type ShellySwitchStatus struct {
	ID     int  `json:"id"`
	Output bool `json:"output"`
}

type Gateway struct {
	mu         sync.RWMutex
	cfg        Config
	configPath string
	client     *http.Client
	safeOutput bool
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	body   strings.Builder
}

type AdminToolStatus struct {
	InterlockName string `json:"interlock_name"`
	IP            string `json:"ip"`
	Port          int    `json:"port"`
	SwitchID      int    `json:"switch_id"`
	Enabled       bool   `json:"enabled"`
	Connected     bool   `json:"connected"`
	Output        bool   `json:"output"`
	Error         string `json:"error,omitempty"`
}

//go:embed web
var embeddedWeb embed.FS

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
	configPath := flag.String("config", "", "path to config.yaml")
	showVersion := flag.Bool("version", false, "print version and exit")
	adminAddr := flag.String("admin", "127.0.0.1:18090", "admin UI listen address; empty disables admin UI")
	flag.Parse()

	if *showVersion {
		fmt.Printf("fbs-interlock-gateway version=%s commit=%s date=%s\n", version, commit, date)
		return
	}

	if *configPath == "" {
		exePath, err := os.Executable()
		if err != nil {
			log.Fatalf("failed to get executable path: %v", err)
		}

		dir := filepath.Dir(exePath)
		*configPath = filepath.Join(dir, "config.yaml")
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %q: %v", *configPath, err)
	}

	for _, tool := range cfg.Tools {
		if tool.Enabled {
			killPort(strconv.Itoa(tool.Port))
		}

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

	log.Printf("fbs-interlock-gateway version=%s commit=%s date=%s config=%s", version, commit, date, *configPath)

	g := &Gateway{
		cfg:        cfg,
		configPath: *configPath,
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

	restartRequested := make(chan struct{}, 1)

	if *adminAddr != "" {
		go g.runAdminServer(ctx, *adminAddr, restartRequested)
	} else {
		log.Printf("admin UI disabled")
	}

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

	select {
	case <-ctx.Done():
		log.Println("shutdown requested")
	case <-restartRequested:
		log.Println("restart requested from admin UI")
		stop()
	}

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

func writeConfigAtomic(path string, cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	backupPath := path + ".bak"
	if oldData, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(backupPath, oldData, 0640)
	}

	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0640); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func validateConfig(cfg Config) error {
	if cfg.Bind == "" {
		cfg.Bind = "0.0.0.0"
	}

	seenPorts := map[int]string{}

	for _, tool := range cfg.Tools {
		if err := validateTool(tool); err != nil {
			return err
		}

		if existing, ok := seenPorts[tool.Port]; ok {
			return fmt.Errorf("duplicate port %d used by %s and %s", tool.Port, existing, tool.InterlockName)
		}

		seenPorts[tool.Port] = tool.InterlockName
	}

	return nil
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

func (g *Gateway) runAdminServer(ctx context.Context, addr string, restartRequested chan<- struct{}) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/config", g.handleAdminConfig)
	mux.HandleFunc("/api/status", g.handleAdminStatus)
	mux.HandleFunc("/api/restart", func(w http.ResponseWriter, r *http.Request) {
		g.handleAdminRestart(w, r, restartRequested)
	})

	webRoot, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		log.Fatalf("failed to load embedded web files: %v", err)
	}

	fileServer := http.FileServer(http.FS(webRoot))
	mux.Handle("/", fileServer)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("admin UI listening on %s", addr)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("admin server error: %v", err)
	}
}

func (g *Gateway) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		g.mu.RLock()
		cfg := g.cfg
		g.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)

	case http.MethodPut:
		var newCfg Config

		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&newCfg); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := validateConfig(newCfg); err != nil {
			http.Error(w, "invalid config: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := writeConfigAtomic(g.configPath, newCfg); err != nil {
			http.Error(w, "failed to write config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		g.mu.Lock()
		g.cfg = newCfg
		g.safeOutput = strings.EqualFold(newCfg.Defaults.SafeStateOnError, "on")
		g.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"saved":true,"restart_required":true}`))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (g *Gateway) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	g.mu.RLock()
	tools := append([]Tool(nil), g.cfg.Tools...)
	g.mu.RUnlock()

	results := make([]AdminToolStatus, 0, len(tools))

	for _, tool := range tools {
		item := AdminToolStatus{
			InterlockName: tool.InterlockName,
			IP:            tool.IP,
			Port:          tool.Port,
			SwitchID:      tool.SwitchID,
			Enabled:       tool.Enabled,
		}

		if !tool.Enabled {
			results = append(results, item)
			continue
		}

		status, err := g.getShellyStatus(tool)
		if err != nil {
			item.Connected = false
			item.Output = g.safeOutput
			item.Error = err.Error()
		} else {
			item.Connected = true
			item.Output = status.Output
		}

		results = append(results, item)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func (g *Gateway) handleAdminRestart(w http.ResponseWriter, r *http.Request, restartRequested chan<- struct{}) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"restart_requested":true}`))

	go func() {
		time.Sleep(500 * time.Millisecond)

		select {
		case restartRequested <- struct{}{}:
		default:
		}
	}()
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

	resp, err := g.doShellyGET(tool, url)
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

	resp, err := g.doShellyGET(tool, url)
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

func killPort(port string) {

	cmd := &exec.Cmd{}

	if runtime.GOOS == "windows" {
		command := fmt.Sprintf("(Get-NetTCPConnection -LocalPort %s).OwningProcess -Force", port)
		cmd = exec.Command("Stop-Process", "-Id", command)
	} else {
		command := fmt.Sprintf("lsof -i tcp:%s | grep LISTEN | awk '{print $2}' | xargs kill -9", port)
		cmd = exec.Command("bash", "-c", command)
	}

	var waitStatus syscall.WaitStatus
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus = exitError.Sys().(syscall.WaitStatus)
			fmt.Printf("Error during killing (exit code: %s)\n", fmt.Appendf(nil, "%d", waitStatus.ExitStatus()))
		}
	} else {
		waitStatus = cmd.ProcessState.Sys().(syscall.WaitStatus)
		fmt.Printf("Port successfully killed (exit code: %s)\n", fmt.Appendf(nil, "%d", waitStatus.ExitStatus()))
	}
}
