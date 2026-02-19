package api

import (
	"context"
	"fmt"

	"github.com/google/go-github/v58/github"
	"github.com/kunlu/git-keys/internal/logger"
)

// GitHubClient implements PlatformClient for GitHub
type GitHubClient struct {
	client *github.Client
	token  string
}

// NewGitHubClient creates a new GitHub API client
func NewGitHubClient(token string) *GitHubClient {
	client := github.NewClient(nil).WithAuthToken(token)
	return &GitHubClient{
		client: client,
		token:  token,
	}
}

// ListKeys lists all SSH keys for the authenticated user
func (c *GitHubClient) ListKeys(ctx context.Context) ([]SSHKey, error) {
	logger.Debug("Listing GitHub SSH keys")

	keys, _, err := c.client.Users.ListKeys(ctx, "", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list GitHub keys: %w", err)
	}

	result := make([]SSHKey, len(keys))
	for i, key := range keys {
		result[i] = SSHKey{
			ID:        fmt.Sprintf("%d", key.GetID()),
			Title:     key.GetTitle(),
			Key:       key.GetKey(),
			CreatedAt: key.GetCreatedAt().String(),
		}
	}

	logger.Info("Found %d SSH keys on GitHub", len(result))
	return result, nil
}

// AddKey adds a new SSH key to GitHub
func (c *GitHubClient) AddKey(ctx context.Context, title, publicKey string) (string, error) {
	logger.Debug("Adding SSH key to GitHub: %s", title)

	key := &github.Key{
		Title: github.String(title),
		Key:   github.String(publicKey),
	}

	created, _, err := c.client.Users.CreateKey(ctx, key)
	if err != nil {
		return "", fmt.Errorf("failed to add GitHub key: %w", err)
	}

	keyID := fmt.Sprintf("%d", created.GetID())
	logger.Info("Added SSH key to GitHub: %s (ID: %s)", title, keyID)
	return keyID, nil
}

// DeleteKey removes an SSH key from GitHub
func (c *GitHubClient) DeleteKey(ctx context.Context, keyID string) error {
	logger.Debug("Deleting GitHub SSH key: %s", keyID)

	var id int64
	fmt.Sscanf(keyID, "%d", &id)

	_, err := c.client.Users.DeleteKey(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete GitHub key: %w", err)
	}

	logger.Info("Deleted SSH key from GitHub: %s", keyID)
	return nil
}

// GetKey retrieves a specific SSH key from GitHub
func (c *GitHubClient) GetKey(ctx context.Context, keyID string) (*SSHKey, error) {
	logger.Debug("Getting GitHub SSH key: %s", keyID)

	var id int64
	fmt.Sscanf(keyID, "%d", &id)

	key, _, err := c.client.Users.GetKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub key: %w", err)
	}

	result := &SSHKey{
		ID:        fmt.Sprintf("%d", key.GetID()),
		Title:     key.GetTitle(),
		Key:       key.GetKey(),
		CreatedAt: key.GetCreatedAt().String(),
	}

	return result, nil
}
