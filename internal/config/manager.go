package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigFileName = ".git-keys.yaml"
	ConfigVersion         = "1.0"
)

// Manager handles configuration file operations
type Manager struct {
	configPath string
}

// NewManager creates a new configuration manager
func NewManager(configPath string) *Manager {
	if configPath == "" {
		configPath = GetDefaultConfigPath()
	}
	return &Manager{configPath: configPath}
}

// GetDefaultConfigPath returns the default config file path
func GetDefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, DefaultConfigFileName)
}

// Load reads the configuration from disk
func (m *Manager) Load() (*Config, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// Save writes the configuration to disk
func (m *Manager) Save(config *Config) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Create parent directory if it doesn't exist
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write with restrictive permissions
	if err := os.WriteFile(m.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Exists checks if the config file exists
func (m *Manager) Exists() bool {
	_, err := os.Stat(m.configPath)
	return err == nil
}

// GetPath returns the config file path
func (m *Manager) GetPath() string {
	return m.configPath
}

// CreateDefault creates a default configuration
func (m *Manager) CreateDefault(machine Machine) *Config {
	return &Config{
		Version:  ConfigVersion,
		Machine:  machine,
		Personas: []Persona{},
		Defaults: Defaults{
			KeyType:       KeyTypeED25519,
			AutoRotate:    false,
			SSHConfigPath: filepath.Join(os.Getenv("HOME"), ".ssh", "config"),
		},
	}
}
