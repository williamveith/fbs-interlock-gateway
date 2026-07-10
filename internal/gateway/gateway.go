package gateway

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/admin"
	"github.com/williamveith/fbs-interlock-gateway/internal/config"
	"github.com/williamveith/fbs-interlock-gateway/internal/fbs"
	"github.com/williamveith/fbs-interlock-gateway/internal/process"
	"github.com/williamveith/fbs-interlock-gateway/internal/shelly"
)

type Gateway struct {
	mu         sync.RWMutex
	cfg        config.Config
	configPath string
	adminAddr  string
	safeOutput bool
	shelly     *shelly.Client
}

func New(cfg config.Config, configPath string, adminAddr string) *Gateway {
	config.ApplyDefaults(&cfg)

	return &Gateway{
		cfg:        cfg,
		configPath: configPath,
		adminAddr:  adminAddr,
		safeOutput: config.SafeOutput(cfg),
		shelly:     shelly.NewClient(time.Duration(cfg.Defaults.TimeoutMS) * time.Millisecond),
	}
}

func (g *Gateway) Run(ctx context.Context) error {
	g.mu.RLock()
	cfg := g.cfg
	adminAddr := g.adminAddr
	g.mu.RUnlock()

	if err := config.ValidateEnabledTools(cfg); err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	restartRequested := make(chan struct{}, 1)
	errCh := make(chan error, len(cfg.Tools)+1)
	var wg sync.WaitGroup

	if adminAddr != "" {
		adminServer := admin.New(adminAddr, g, g.shelly, restartRequested)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := adminServer.Run(runCtx); err != nil {
				errCh <- err
			}
		}()
	} else {
		log.Printf("admin UI disabled")
	}

	fbsServer := fbs.NewServer(cfg.Bind, g.SafeOutput(), g.shelly)
	enabledCount := 0

	for _, tool := range cfg.Tools {
		if !tool.Enabled {
			log.Printf("tool=%s port=%d disabled; skipping", tool.InterlockName, tool.Port)
			continue
		}

		enabledCount++

		if err := process.KillPort(tool.Port); err != nil {
			log.Printf("warning: failed to clear port %d for tool=%s: %v", tool.Port, tool.InterlockName, err)
		}

		tool := tool
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fbsServer.RunToolServer(runCtx, tool); err != nil {
				errCh <- err
			}
		}()
	}

	log.Printf("fbs-interlock-gateway started with %d enabled tool(s)", enabledCount)

	select {
	case <-ctx.Done():
		log.Println("shutdown requested")
	case <-restartRequested:
		log.Println("restart requested from admin UI")
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	}

	cancel()
	wg.Wait()
	log.Println("shutdown complete")
	return nil
}

func (g *Gateway) ConfigSnapshot() config.Config {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.cfg
}

func (g *Gateway) SafeOutput() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.safeOutput
}

func (g *Gateway) UpdateConfig(newCfg config.Config) error {
	config.ApplyDefaults(&newCfg)

	if err := config.Validate(newCfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	if err := config.WriteAtomic(g.configPath, newCfg); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	g.mu.Lock()
	g.cfg = newCfg
	g.safeOutput = config.SafeOutput(newCfg)
	g.mu.Unlock()

	return nil
}
