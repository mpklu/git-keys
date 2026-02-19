package sshkey

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
)

// Manager handles SSH key operations
type Manager struct {
	keysDir string
}

// NewManager creates a new SSH key manager
func NewManager(keysDir string) *Manager {
	if keysDir == "" {
		home, _ := os.UserHomeDir()
		keysDir = filepath.Join(home, ".ssh")
	}
	return &Manager{keysDir: keysDir}
}

// GenerateKey generates a new SSH key pair
func (m *Manager) GenerateKey(keyType config.KeyType, comment string, outputPath string) error {
	logger.Debug("Generating %s key with comment: %s", keyType, comment)

	// Ensure keys directory exists
	if err := os.MkdirAll(m.keysDir, 0700); err != nil {
		return fmt.Errorf("failed to create keys directory: %w", err)
	}

	fullPath := filepath.Join(m.keysDir, outputPath)

	// Build ssh-keygen command
	var args []string
	switch keyType {
	case config.KeyTypeED25519:
		args = []string{"-t", "ed25519", "-f", fullPath, "-N", "", "-C", comment}
	case config.KeyTypeRSA:
		args = []string{"-t", "rsa", "-b", "4096", "-f", fullPath, "-N", "", "-C", comment}
	default:
		return fmt.Errorf("unsupported key type: %s", keyType)
	}

	cmd := exec.Command("ssh-keygen", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate key: %w\nOutput: %s", err, string(output))
	}

	// Set correct permissions
	if err := os.Chmod(fullPath, 0600); err != nil {
		return fmt.Errorf("failed to set private key permissions: %w", err)
	}

	logger.Info("Generated %s key: %s", keyType, fullPath)
	return nil
}

// GetFingerprint returns the fingerprint of a public key file
func (m *Manager) GetFingerprint(publicKeyPath string) (string, error) {
	fullPath := filepath.Join(m.keysDir, publicKeyPath)
	if !strings.HasSuffix(fullPath, ".pub") {
		fullPath += ".pub"
	}

	cmd := exec.Command("ssh-keygen", "-lf", fullPath)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get fingerprint: %w", err)
	}

	// Parse output like: "256 SHA256:xxxxx... comment (ED25519)"
	parts := strings.Fields(string(output))
	if len(parts) >= 2 {
		return parts[1], nil
	}

	return "", fmt.Errorf("unexpected ssh-keygen output format")
}

// GetPublicKey reads the public key content
func (m *Manager) GetPublicKey(publicKeyPath string) (string, error) {
	fullPath := filepath.Join(m.keysDir, publicKeyPath)
	if !strings.HasSuffix(fullPath, ".pub") {
		fullPath += ".pub"
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read public key: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// KeyExists checks if a key file exists
func (m *Manager) KeyExists(keyPath string) bool {
	fullPath := filepath.Join(m.keysDir, keyPath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// DeleteKey removes a key pair
func (m *Manager) DeleteKey(keyPath string) error {
	privateKey := filepath.Join(m.keysDir, keyPath)
	publicKey := privateKey + ".pub"

	// Remove private key
	if err := os.Remove(privateKey); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove private key: %w", err)
	}

	// Remove public key
	if err := os.Remove(publicKey); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove public key: %w", err)
	}

	logger.Info("Deleted key: %s", keyPath)
	return nil
}

// BuildKeyComment creates a standardized key comment
func BuildKeyComment(platform config.PlatformType, account, machineName string) string {
	return fmt.Sprintf("git-keys:%s:%s:%s", platform, account, machineName)
}

// BuildKeyFileName creates a standardized key file name
func BuildKeyFileName(platform config.PlatformType, account string, keyType config.KeyType) string {
	return fmt.Sprintf("git-keys-%s-%s-%s", platform, account, keyType)
}
