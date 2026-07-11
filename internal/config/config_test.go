package config

import (
	"strings"
	"testing"
)

func validTool(name string, port int, enabled bool) Tool {
	return Tool{
		InterlockName: name,
		IP:            "127.0.0.1",
		Port:          port,
		SwitchID:      0,
		Enabled:       enabled,
	}
}

func TestApplyDefaults(t *testing.T) {
	var cfg Config
	ApplyDefaults(&cfg)

	if cfg.Bind != "0.0.0.0" {
		t.Fatalf("Bind = %q, want 0.0.0.0", cfg.Bind)
	}
	if cfg.Defaults.TimeoutMS != 800 {
		t.Fatalf("TimeoutMS = %d, want 800", cfg.Defaults.TimeoutMS)
	}
	if cfg.Defaults.SafeStateOnError != "off" {
		t.Fatalf("SafeStateOnError = %q, want off", cfg.Defaults.SafeStateOnError)
	}
}

func TestValidateRejectsDuplicatePorts(t *testing.T) {
	cfg := Config{Tools: []Tool{
		validTool("tool-a", 8081, true),
		validTool("tool-b", 8081, true),
	}}

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "duplicate port 8081") {
		t.Fatalf("Validate() error = %v, want duplicate-port error", err)
	}
}

func TestValidateEnabledToolsIgnoresDisabledTool(t *testing.T) {
	cfg := Config{Tools: []Tool{
		validTool("tool-a", 8081, true),
		validTool("disabled-copy", 8081, false),
	}}

	if err := ValidateEnabledTools(cfg); err != nil {
		t.Fatalf("ValidateEnabledTools() error = %v", err)
	}
}

func TestValidateToolRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name string
		tool Tool
	}{
		{name: "missing name", tool: Tool{IP: "host", Port: 8081}},
		{name: "missing ip", tool: Tool{InterlockName: "tool", Port: 8081}},
		{name: "zero port", tool: Tool{InterlockName: "tool", IP: "host", Port: 0}},
		{name: "high port", tool: Tool{InterlockName: "tool", IP: "host", Port: 65536}},
		{name: "negative switch", tool: Tool{InterlockName: "tool", IP: "host", Port: 8081, SwitchID: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateTool(tt.tool); err == nil {
				t.Fatal("ValidateTool() returned nil error")
			}
		})
	}
}
