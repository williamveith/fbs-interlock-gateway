package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	MinToolPort = 8081
	MaxToolPort = 8981
)

type Config struct {
	Bind     string   `yaml:"bind" json:"bind"`
	Defaults Defaults `yaml:"defaults" json:"defaults"`
	Tools    []Tool   `yaml:"tools" json:"tools"`
}

type ShellyTLSConfig struct {
	ServerCAFile   string `yaml:"server_ca_file" json:"server_ca_file"`
	ClientCertFile string `yaml:"client_cert_file" json:"client_cert_file"`
	ClientKeyFile  string `yaml:"client_key_file" json:"client_key_file"`
}

type Defaults struct {
	TimeoutMS        int             `yaml:"timeout_ms" json:"timeout_ms"`
	SafeStateOnError string          `yaml:"safe_state_on_error" json:"safe_state_on_error"`
	ShellyTLS        ShellyTLSConfig `yaml:"shelly_tls,omitempty" json:"shelly_tls"`
}

type Tool struct {
	InterlockName string  `yaml:"interlock_name" json:"interlock_name"`
	IP            string  `yaml:"ip" json:"ip"`
	Protocol      string  `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	Port          int     `yaml:"port" json:"port"`
	SwitchID      int     `yaml:"switch_id" json:"switch_id"`
	Username      *string `yaml:"username" json:"username"`
	Password      *string `yaml:"password" json:"password"`
	Enabled       bool    `yaml:"enabled" json:"enabled"`
}

func Clone(cfg Config) Config {
	cloned := cfg
	cloned.Tools = make([]Tool, len(cfg.Tools))

	for i, tool := range cfg.Tools {
		cloned.Tools[i] = tool
		cloned.Tools[i].Username = cloneString(tool.Username)
		cloned.Tools[i].Password = cloneString(tool.Password)
	}

	return cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}

func Load(path string) (Config, error) {
	var cfg Config

	absoluteConfigPath, err := filepath.Abs(path)
	if err != nil {
		return cfg, fmt.Errorf(
			"resolve config path %q: %w",
			path,
			err,
		)
	}

	data, err := os.ReadFile(absoluteConfigPath)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	configDir := filepath.Dir(absoluteConfigPath)

	cfg.Defaults.ShellyTLS.ServerCAFile = resolveRelativePath(
		configDir,
		cfg.Defaults.ShellyTLS.ServerCAFile,
	)

	cfg.Defaults.ShellyTLS.ClientCertFile = resolveRelativePath(
		configDir,
		cfg.Defaults.ShellyTLS.ClientCertFile,
	)

	cfg.Defaults.ShellyTLS.ClientKeyFile = resolveRelativePath(
		configDir,
		cfg.Defaults.ShellyTLS.ClientKeyFile,
	)

	return cfg, nil
}

func resolveRelativePath(baseDir string, path string) string {
	path = strings.TrimSpace(path)

	if path == "" || filepath.IsAbs(path) {
		return path
	}

	return filepath.Clean(filepath.Join(baseDir, path))
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
			return fmt.Errorf(
				"duplicate port %d used by %s and %s",
				tool.Port,
				existing,
				tool.InterlockName,
			)
		}

		seenPorts[tool.Port] = tool.InterlockName
	}

	return validateTLSConfig(cfg, false)
}

func ValidateEnabledTools(cfg Config) error {
	ApplyDefaults(&cfg)

	seenPorts := map[int]string{}
	for _, tool := range cfg.Tools {
		if !tool.Enabled {
			continue
		}

		if err := ValidateTool(tool); err != nil {
			return err
		}

		if existing, ok := seenPorts[tool.Port]; ok {
			return fmt.Errorf(
				"duplicate enabled port %d used by %s and %s",
				tool.Port,
				existing,
				tool.InterlockName,
			)
		}

		seenPorts[tool.Port] = tool.InterlockName
	}

	return validateTLSConfig(cfg, true)
}

func ValidateTool(tool Tool) error {
	if strings.TrimSpace(tool.InterlockName) == "" {
		return errors.New("missing interlock_name")
	}

	if tool.Port < MinToolPort || tool.Port > MaxToolPort {
		return fmt.Errorf(
			"tool %q port must be between %d and %d",
			tool.InterlockName,
			MinToolPort,
			MaxToolPort,
		)
	}

	if strings.TrimSpace(tool.IP) == "" {
		return errors.New("missing ip")
	}

	if strings.Contains(tool.IP, "://") {
		return fmt.Errorf(
			"tool %q ip must not include a URL scheme",
			tool.InterlockName,
		)
	}

	if tool.SwitchID < 0 {
		return fmt.Errorf("invalid switch_id %d", tool.SwitchID)
	}

	return validateToolProtocol(tool)
}

// ToolProtocol returns the normalized RPC protocol for a tool. An omitted
// protocol remains backward-compatible with existing configurations and uses
// plain HTTP.
func ToolProtocol(tool Tool) string {
	protocol := strings.ToLower(strings.TrimSpace(tool.Protocol))
	if protocol == "" {
		return "http"
	}

	return protocol
}

func validateToolProtocol(tool Tool) error {
	switch ToolProtocol(tool) {
	case "http", "https":
		return nil
	default:
		return fmt.Errorf(
			"tool %q has invalid protocol %q; expected http or https",
			tool.InterlockName,
			tool.Protocol,
		)
	}
}

func validateTLSConfig(cfg Config, enabledOnly bool) error {
	httpsRequired := false

	for _, tool := range cfg.Tools {
		if enabledOnly && !tool.Enabled {
			continue
		}

		if ToolProtocol(tool) == "https" {
			httpsRequired = true
			break
		}
	}

	if !httpsRequired {
		return nil
	}

	tlsConfig := cfg.Defaults.ShellyTLS

	if strings.TrimSpace(tlsConfig.ServerCAFile) == "" {
		return errors.New("defaults.shelly_tls.server_ca_file is required when HTTPS is enabled")
	}

	if strings.TrimSpace(tlsConfig.ClientCertFile) == "" {
		return errors.New("defaults.shelly_tls.client_cert_file is required when HTTPS is enabled")
	}

	if strings.TrimSpace(tlsConfig.ClientKeyFile) == "" {
		return errors.New("defaults.shelly_tls.client_key_file is required when HTTPS is enabled")
	}

	return nil
}
