package api

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kunlu/git-keys/internal/logger"
)

// PlatformClient defines the interface for interacting with git platforms
type PlatformClient interface {
	ListKeys(ctx context.Context) ([]SSHKey, error)
	AddKey(ctx context.Context, title, publicKey string) (string, error)
	DeleteKey(ctx context.Context, keyID string) error
	GetKey(ctx context.Context, keyID string) (*SSHKey, error)
}

// SSHKey represents an SSH key on a platform
type SSHKey struct {
	ID          string
	Title       string
	Key         string
	Fingerprint string
	CreatedAt   string
}

// TokenManager handles API token storage and retrieval
type TokenManager struct {
	keychainService string
}

// NewTokenManager creates a new token manager
func NewTokenManager(service string) *TokenManager {
	return &TokenManager{keychainService: service}
}

// GetToken retrieves the API token from keychain
func (tm *TokenManager) GetToken(account string) (string, error) {
	logger.Debug("Retrieving token for account: %s", account)

	cmd := exec.Command("security", "find-generic-password",
		"-s", tm.keychainService,
		"-a", account,
		"-w")

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("token not found in keychain: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// SetToken stores the API token in keychain
func (tm *TokenManager) SetToken(account, token string) error {
	logger.Debug("Storing token for account: %s", account)

	cmd := exec.Command("security", "add-generic-password",
		"-s", tm.keychainService,
		"-a", account,
		"-w", token,
		"-U")

	if err := cmd.Run(); err != nil {
		// Try without -U if update fails
		cmd = exec.Command("security", "add-generic-password",
			"-s", tm.keychainService,
			"-a", account,
			"-w", token)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to store token: %w", err)
		}
	}

	logger.Info("Token stored for account: %s", account)
	return nil
}
