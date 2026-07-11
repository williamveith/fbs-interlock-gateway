package admin

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
	"github.com/williamveith/fbs-interlock-gateway/internal/shelly"
)

type ConfigStore interface {
	ConfigSnapshot() config.Config
	UpdateConfig(newCfg config.Config) error
	SafeOutput() bool
}

type StatusClient interface {
	GetStatus(ctx context.Context, tool config.Tool) (shelly.SwitchStatus, error)
}

type Server struct {
	addr             string
	store            ConfigStore
	statusClient     StatusClient
	restartRequested chan<- struct{}
}

type ToolStatus struct {
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

func New(addr string, store ConfigStore, statusClient StatusClient, restartRequested chan<- struct{}) *Server {
	return &Server{
		addr:             addr,
		store:            store,
		statusClient:     statusClient,
		restartRequested: restartRequested,
	}
}

func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/restart", s.handleRestart)

	webRoot, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return fmt.Errorf("failed to load embedded web files: %w", err)
	}

	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	server := &http.Server{
		Addr:              s.addr,
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

	log.Printf("admin UI listening on %s", s.addr)

	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("admin server error: %w", err)
	}

	return nil
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.ConfigSnapshot())

	case http.MethodPut:
		var newCfg config.Config

		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&newCfg); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := s.store.UpdateConfig(newCfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{
			"saved":            true,
			"restart_required": true,
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := s.store.ConfigSnapshot()
	results := make([]ToolStatus, len(cfg.Tools))

	var wg sync.WaitGroup

	for i, tool := range cfg.Tools {
		results[i] = ToolStatus{
			InterlockName: tool.InterlockName,
			IP:            tool.IP,
			Port:          tool.Port,
			SwitchID:      tool.SwitchID,
			Enabled:       tool.Enabled,
		}

		if !tool.Enabled {
			continue
		}

		wg.Add(1)

		go func(index int, tool config.Tool) {
			defer wg.Done()

			status, err := s.statusClient.GetStatus(
				r.Context(),
				tool,
			)

			if err != nil {
				results[index].Connected = false
				results[index].Output = s.store.SafeOutput()
				results[index].Error = err.Error()
				return
			}

			results[index].Connected = true
			results[index].Output = status.Output
		}(i, tool)
	}

	wg.Wait()

	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{
		"restart_requested": true,
	})

	go func() {
		time.Sleep(500 * time.Millisecond)

		select {
		case s.restartRequested <- struct{}{}:
		default:
		}
	}()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("failed to write JSON response: %v", err)
	}
}
