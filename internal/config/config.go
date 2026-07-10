package config

import (
	"fmt"
	"os"
	"strings"

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

func Load(path string) (Config, error) {
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

func WriteAtomic(path string, cfg Config) error {
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

func ApplyDefaults(cfg *Config) {
	if cfg.Bind == "" {
		cfg.Bind = "0.0.0.0"
	}

	if cfg.Defaults.TimeoutMS <= 0 {
		cfg.Defaults.TimeoutMS = 800
	}

	if cfg.Defaults.SafeStateOnError == "" {
		cfg.Defaults.SafeStateOnError = "off"
	}
}

func SafeOutput(cfg Config) bool {
	return strings.EqualFold(cfg.Defaults.SafeStateOnError, "on")
}

func Validate(cfg Config) error {
	ApplyDefaults(&cfg)

	seenPorts := map[int]string{}
	for _, tool := range cfg.Tools {
		if err := ValidateTool(tool); err != nil {
			return err
		}

		if existing, ok := seenPorts[tool.Port]; ok {
			return fmt.Errorf("duplicate port %d used by %s and %s", tool.Port, existing, tool.InterlockName)
		}

		seenPorts[tool.Port] = tool.InterlockName
	}

	return nil
}

func ValidateEnabledTools(cfg Config) error {
	seenPorts := map[int]string{}
	for _, tool := range cfg.Tools {
		if !tool.Enabled {
			continue
		}

		if err := ValidateTool(tool); err != nil {
			return err
		}

		if existing, ok := seenPorts[tool.Port]; ok {
			return fmt.Errorf("duplicate enabled port %d used by %s and %s", tool.Port, existing, tool.InterlockName)
		}

		seenPorts[tool.Port] = tool.InterlockName
	}

	return nil
}

func ValidateTool(t Tool) error {
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
