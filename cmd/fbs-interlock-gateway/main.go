package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
	"github.com/williamveith/fbs-interlock-gateway/internal/gateway"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

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

		*configPath = filepath.Join(filepath.Dir(exePath), "config.yaml")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %q: %v", *configPath, err)
	}

	config.ApplyDefaults(&cfg)

	log.Printf("fbs-interlock-gateway version=%s commit=%s date=%s config=%s", version, commit, date, *configPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := gateway.New(cfg, *configPath, *adminAddr)
	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
