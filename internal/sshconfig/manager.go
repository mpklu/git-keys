package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
)

const (
	managedBlockStart = "# BEGIN git-keys managed block -"
	managedBlockEnd   = "# END git-keys managed block"
)

// Manager handles SSH config file operations
type Manager struct {
	configPath string
}

// NewManager creates a new SSH config manager
func NewManager(configPath string) *Manager {
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".ssh", "config")
	}
	return &Manager{configPath: configPath}
}

// Entry represents a Host entry in SSH config
type Entry struct {
	Host         string
	HostName     string
	User         string
	IdentityFile string
	Extra        map[string]string
}

// EnsureConfigExists creates the SSH config file if it doesn't exist
func (m *Manager) EnsureConfigExists() error {
	// Create .ssh directory if needed
	sshDir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Create config file if it doesn't exist
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		f, err := os.OpenFile(m.configPath, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("failed to create SSH config: %w", err)
		}
		f.Close()
		logger.Info("Created SSH config file: %s", m.configPath)
	}

	return nil
}

// GetManagedBlockID returns the block ID for a persona/platform
func GetManagedBlockID(persona string, platform config.PlatformType, account string) string {
	return fmt.Sprintf("%s-%s-%s", persona, platform, account)
}

// AddOrUpdateEntry adds or updates a managed block in SSH config
func (m *Manager) AddOrUpdateEntry(blockID string, entries []Entry) error {
	if err := m.EnsureConfigExists(); err != nil {
		return err
	}

	// Read existing config
	content, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to read SSH config: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	newLines := m.removeManagedBlock(lines, blockID)

	// Add new managed block
	blockLines := m.buildManagedBlock(blockID, entries)
	newLines = append(newLines, blockLines...)

	// Write back
	newContent := strings.Join(newLines, "\n")
	if err := os.WriteFile(m.configPath, []byte(newContent), 0600); err != nil {
		return fmt.Errorf("failed to write SSH config: %w", err)
	}

	logger.Info("Updated SSH config managed block: %s", blockID)
	return nil
}

// removeManagedBlock removes a specific managed block from lines
func (m *Manager) removeManagedBlock(lines []string, blockID string) []string {
	startMarker := fmt.Sprintf("%s %s", managedBlockStart, blockID)
	var result []string
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, startMarker) {
			inBlock = true
			continue
		}

		if inBlock && strings.HasPrefix(trimmed, managedBlockEnd) {
			inBlock = false
			continue
		}

		if !inBlock {
			result = append(result, line)
		}
	}

	return result
}

// buildManagedBlock creates a managed block with entries
func (m *Manager) buildManagedBlock(blockID string, entries []Entry) []string {
	var lines []string

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("%s %s", managedBlockStart, blockID))

	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("Host %s", entry.Host))
		if entry.HostName != "" {
			lines = append(lines, fmt.Sprintf("  HostName %s", entry.HostName))
		}
		if entry.User != "" {
			lines = append(lines, fmt.Sprintf("  User %s", entry.User))
		}
		if entry.IdentityFile != "" {
			lines = append(lines, fmt.Sprintf("  IdentityFile %s", entry.IdentityFile))
		}
		for key, value := range entry.Extra {
			lines = append(lines, fmt.Sprintf("  %s %s", key, value))
		}
		lines = append(lines, "")
	}

	lines = append(lines, managedBlockEnd)
	lines = append(lines, "")

	return lines
}

// BackupConfig creates a backup of the SSH config file
func (m *Manager) BackupConfig() (string, error) {
	content, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read SSH config: %w", err)
	}

	backupPath := m.configPath + ".backup"
	if err := os.WriteFile(backupPath, content, 0600); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	logger.Info("Created SSH config backup: %s", backupPath)
	return backupPath, nil
}
