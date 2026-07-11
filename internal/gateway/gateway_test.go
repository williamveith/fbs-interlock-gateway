package gateway

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
)

func testConfig(safeState string) config.Config {
	return config.Config{
		Bind: "127.0.0.1",
		Defaults: config.Defaults{
			TimeoutMS:        800,
			SafeStateOnError: safeState,
		},
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-01",
				IP:            "192.0.2.10",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	gateway := New(config.Config{}, "/tmp/config.yaml", "")

	snapshot := gateway.ConfigSnapshot()

	if snapshot.Bind != "0.0.0.0" {
		t.Fatalf(
			"expected default bind 0.0.0.0, got %q",
			snapshot.Bind,
		)
	}

	if snapshot.Defaults.TimeoutMS != 800 {
		t.Fatalf(
			"expected default timeout 800, got %d",
			snapshot.Defaults.TimeoutMS,
		)
	}

	if snapshot.Defaults.SafeStateOnError != "off" {
		t.Fatalf(
			"expected default safe state off, got %q",
			snapshot.Defaults.SafeStateOnError,
		)
	}

	if gateway.SafeOutput() {
		t.Fatal("expected default safe output to be false")
	}

	if gateway.shelly == nil {
		t.Fatal("expected Shelly client to be initialized")
	}
}

func TestNewSetsSafeOutputOn(t *testing.T) {
	cfg := testConfig("on")

	gateway := New(cfg, "/tmp/config.yaml", "")

	if !gateway.SafeOutput() {
		t.Fatal("expected safe output to be true")
	}
}

func TestConfigSnapshotReturnsCurrentConfig(t *testing.T) {
	cfg := testConfig("off")

	gateway := New(cfg, "/tmp/config.yaml", "")

	snapshot := gateway.ConfigSnapshot()

	if !reflect.DeepEqual(snapshot, cfg) {
		t.Fatalf(
			"unexpected config snapshot\nexpected: %#v\nactual:   %#v",
			cfg,
			snapshot,
		)
	}
}

func TestUpdateConfigWritesFileAndUpdatesState(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	originalContents := []byte("original configuration\n")

	if err := os.WriteFile(configPath, originalContents, 0640); err != nil {
		t.Fatalf("failed to create original config: %v", err)
	}

	gateway := New(
		testConfig("off"),
		configPath,
		"",
	)

	newCfg := config.Config{
		Defaults: config.Defaults{
			SafeStateOnError: "on",
		},
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-02",
				IP:            "192.0.2.20",
				Port:          8082,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	if err := gateway.UpdateConfig(newCfg); err != nil {
		t.Fatalf("UpdateConfig returned an error: %v", err)
	}

	snapshot := gateway.ConfigSnapshot()

	if snapshot.Bind != "0.0.0.0" {
		t.Fatalf(
			"expected default bind 0.0.0.0, got %q",
			snapshot.Bind,
		)
	}

	if snapshot.Defaults.TimeoutMS != 800 {
		t.Fatalf(
			"expected default timeout 800, got %d",
			snapshot.Defaults.TimeoutMS,
		)
	}

	if snapshot.Defaults.SafeStateOnError != "on" {
		t.Fatalf(
			"expected safe state on, got %q",
			snapshot.Defaults.SafeStateOnError,
		)
	}

	if !gateway.SafeOutput() {
		t.Fatal("expected safe output to be true after update")
	}

	savedCfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if !reflect.DeepEqual(savedCfg, snapshot) {
		t.Fatalf(
			"saved config does not match gateway state\nsaved:    %#v\ngateway:  %#v",
			savedCfg,
			snapshot,
		)
	}

	backupContents, err := os.ReadFile(configPath + ".bak")
	if err != nil {
		t.Fatalf("failed to read config backup: %v", err)
	}

	if !reflect.DeepEqual(backupContents, originalContents) {
		t.Fatalf(
			"unexpected backup contents\nexpected: %q\nactual:   %q",
			originalContents,
			backupContents,
		)
	}
}

func TestUpdateConfigRejectsInvalidConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	originalContents := []byte("original configuration\n")

	if err := os.WriteFile(configPath, originalContents, 0640); err != nil {
		t.Fatalf("failed to create original config: %v", err)
	}

	originalCfg := testConfig("off")
	gateway := New(originalCfg, configPath, "")

	invalidCfg := config.Config{
		Tools: []config.Tool{
			{
				InterlockName: "EQU-TEST-01",
				IP:            "192.0.2.10",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
			{
				InterlockName: "EQU-TEST-02",
				IP:            "192.0.2.11",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	err := gateway.UpdateConfig(invalidCfg)
	if err == nil {
		t.Fatal("expected invalid config to be rejected")
	}

	if !strings.Contains(err.Error(), "duplicate port") {
		t.Fatalf(
			"expected duplicate port error, got %q",
			err,
		)
	}

	snapshot := gateway.ConfigSnapshot()

	if !reflect.DeepEqual(snapshot, originalCfg) {
		t.Fatalf(
			"gateway config changed after rejected update\nexpected: %#v\nactual:   %#v",
			originalCfg,
			snapshot,
		)
	}

	currentContents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	if !reflect.DeepEqual(currentContents, originalContents) {
		t.Fatalf(
			"config file changed after rejected update\nexpected: %q\nactual:   %q",
			originalContents,
			currentContents,
		)
	}
}

func TestUpdateConfigReturnsWriteError(t *testing.T) {
	tempDir := t.TempDir()

	configPath := filepath.Join(
		tempDir,
		"directory-that-does-not-exist",
		"config.yaml",
	)

	originalCfg := testConfig("off")
	gateway := New(originalCfg, configPath, "")

	newCfg := testConfig("on")
	newCfg.Tools[0].Port = 8082

	err := gateway.UpdateConfig(newCfg)
	if err == nil {
		t.Fatal("expected config write to fail")
	}

	if !strings.Contains(err.Error(), "failed to write config") {
		t.Fatalf(
			"expected config write error, got %q",
			err,
		)
	}

	snapshot := gateway.ConfigSnapshot()

	if !reflect.DeepEqual(snapshot, originalCfg) {
		t.Fatalf(
			"gateway config changed after write failure\nexpected: %#v\nactual:   %#v",
			originalCfg,
			snapshot,
		)
	}

	if gateway.SafeOutput() {
		t.Fatal("safe output changed after failed config write")
	}
}

func TestRunRejectsInvalidEnabledTool(t *testing.T) {
	cfg := config.Config{
		Tools: []config.Tool{
			{
				InterlockName: "EQU-BROKEN-01",
				Port:          8081,
				SwitchID:      0,
				Enabled:       true,
			},
		},
	}

	gateway := New(cfg, "/tmp/config.yaml", "")

	err := gateway.Run(context.Background())
	if err == nil {
		t.Fatal("expected Run to reject invalid enabled tool")
	}

	if !strings.Contains(err.Error(), "missing ip") {
		t.Fatalf(
			"expected missing IP error, got %q",
			err,
		)
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	gateway := New(config.Config{}, "/tmp/config.yaml", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := gateway.Run(ctx); err != nil {
		t.Fatalf(
			"expected clean shutdown after context cancellation, got %v",
			err,
		)
	}
}

func TestRunSkipsInvalidDisabledTool(t *testing.T) {
	cfg := config.Config{
		Tools: []config.Tool{
			{
				InterlockName: "",
				IP:            "",
				Port:          0,
				SwitchID:      -1,
				Enabled:       false,
			},
		},
	}

	gateway := New(cfg, "/tmp/config.yaml", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := gateway.Run(ctx); err != nil {
		t.Fatalf(
			"expected disabled invalid tool to be skipped, got %v",
			err,
		)
	}
}

func TestConcurrentConfigReadsAndUpdates(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	initialCfg := testConfig("off")

	if err := config.WriteAtomic(configPath, initialCfg); err != nil {
		t.Fatalf("failed to create initial config: %v", err)
	}

	gateway := New(initialCfg, configPath, "")

	const readerCount = 8
	const iterations = 100

	var waitGroup sync.WaitGroup

	for reader := 0; reader < readerCount; reader++ {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for iteration := 0; iteration < iterations; iteration++ {
				snapshot := gateway.ConfigSnapshot()

				if snapshot.Bind == "" {
					t.Error("config snapshot unexpectedly had empty bind")
				}

				_ = gateway.SafeOutput()
			}
		}()
	}

	for iteration := 0; iteration < iterations; iteration++ {
		safeState := "off"
		if iteration%2 == 0 {
			safeState = "on"
		}

		newCfg := testConfig(safeState)
		newCfg.Tools[0].Port = 8081 + iteration

		if err := gateway.UpdateConfig(newCfg); err != nil {
			t.Fatalf(
				"config update %d failed: %v",
				iteration,
				err,
			)
		}
	}

	waitGroup.Wait()
}
