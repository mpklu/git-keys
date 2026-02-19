package config

import (
	"fmt"
	"time"
)

// Config represents the git-keys configuration file
type Config struct {
	Version  string    `yaml:"version"`
	Machine  Machine   `yaml:"machine"`
	Personas []Persona `yaml:"personas"`
	Defaults Defaults  `yaml:"defaults,omitempty"`
}

// Machine represents the local machine identity
type Machine struct {
	ID        string `yaml:"id"`   // Hardware UUID
	Name      string `yaml:"name"` // Human-readable name
	OS        string `yaml:"os"`   // Operating system
	OSVersion string `yaml:"os_version,omitempty"`
}

// Persona represents a git identity (personal, work, etc.)
type Persona struct {
	Name      string     `yaml:"name"`  // e.g., "personal", "work"
	Email     string     `yaml:"email"` // Git commit email
	Platforms []Platform `yaml:"platforms"`
}

// Platform represents a git hosting platform configuration
type Platform struct {
	Type    PlatformType `yaml:"type"`               // "github" or "gitlab"
	Account string       `yaml:"account"`            // Username or organization
	BaseURL string       `yaml:"base_url,omitempty"` // For self-hosted GitLab
	Keys    []KeyConfig  `yaml:"keys,omitempty"`     // Managed keys
}

// PlatformType is the type of git hosting platform
type PlatformType string

const (
	PlatformGitHub PlatformType = "github"
	PlatformGitLab PlatformType = "gitlab"
)

// KeyConfig represents a managed SSH key
type KeyConfig struct {
	Type        KeyType   `yaml:"type"` // "ed25519" or "rsa"
	CreatedAt   time.Time `yaml:"created_at"`
	ExpiresAt   time.Time `yaml:"expires_at"`
	Fingerprint string    `yaml:"fingerprint"`
	LocalPath   string    `yaml:"local_path"`          // Path to private key
	RemoteID    string    `yaml:"remote_id,omitempty"` // Platform's key ID
	Status      KeyStatus `yaml:"status"`
}

// KeyType represents the SSH key algorithm
type KeyType string

const (
	KeyTypeED25519 KeyType = "ed25519"
	KeyTypeRSA     KeyType = "rsa"
)

// KeyStatus represents the state of a key
type KeyStatus string

const (
	KeyStatusActive  KeyStatus = "active"
	KeyStatusExpired KeyStatus = "expired"
	KeyStatusRevoked KeyStatus = "revoked"
	KeyStatusPending KeyStatus = "pending" // Not yet uploaded
)

// Defaults represents default configuration values
type Defaults struct {
	KeyType       KeyType       `yaml:"key_type,omitempty"`
	KeyExpiration time.Duration `yaml:"key_expiration,omitempty"`
	AutoRotate    bool          `yaml:"auto_rotate,omitempty"`
	SSHConfigPath string        `yaml:"ssh_config_path,omitempty"`
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}
	if c.Machine.ID == "" {
		return fmt.Errorf("machine.id is required")
	}
	if len(c.Personas) == 0 {
		return fmt.Errorf("at least one persona is required")
	}

	for i, persona := range c.Personas {
		if persona.Name == "" {
			return fmt.Errorf("persona[%d].name is required", i)
		}
		if persona.Email == "" {
			return fmt.Errorf("persona[%d].email is required", i)
		}
		if len(persona.Platforms) == 0 {
			return fmt.Errorf("persona[%d] must have at least one platform", i)
		}
	}

	return nil
}

// FindPersona finds a persona by name
func (c *Config) FindPersona(name string) *Persona {
	for i := range c.Personas {
		if c.Personas[i].Name == name {
			return &c.Personas[i]
		}
	}
	return nil
}

// FindPlatform finds a platform within a persona
func (p *Persona) FindPlatform(platformType PlatformType, account string) *Platform {
	for i := range p.Platforms {
		if p.Platforms[i].Type == platformType && p.Platforms[i].Account == account {
			return &p.Platforms[i]
		}
	}
	return nil
}

// GetActiveKey returns the active key for this platform
func (p *Platform) GetActiveKey() *KeyConfig {
	for i := range p.Keys {
		if p.Keys[i].Status == KeyStatusActive {
			return &p.Keys[i]
		}
	}
	return nil
}

// GetExpiredKeys returns all expired keys
func (p *Platform) GetExpiredKeys() []KeyConfig {
	var expired []KeyConfig
	now := time.Now()
	for _, key := range p.Keys {
		if key.Status == KeyStatusActive && key.ExpiresAt.Before(now) {
			expired = append(expired, key)
		}
	}
	return expired
}
