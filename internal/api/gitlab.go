package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kunlu/git-keys/internal/logger"
)

// GitLabClient implements PlatformClient for GitLab
type GitLabClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewGitLabClient creates a new GitLab API client
func NewGitLabClient(baseURL, token string) *GitLabClient {
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &GitLabClient{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{},
	}
}

type gitlabKey struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Key       string `json:"key"`
	CreatedAt string `json:"created_at"`
}

// ListKeys lists all SSH keys for the authenticated user
func (c *GitLabClient) ListKeys(ctx context.Context) ([]SSHKey, error) {
	logger.Debug("Listing GitLab SSH keys")

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v4/user/keys", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list GitLab keys: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(body))
	}

	var keys []gitlabKey
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	result := make([]SSHKey, len(keys))
	for i, key := range keys {
		result[i] = SSHKey{
			ID:        fmt.Sprintf("%d", key.ID),
			Title:     key.Title,
			Key:       key.Key,
			CreatedAt: key.CreatedAt,
		}
	}

	logger.Info("Found %d SSH keys on GitLab", len(result))
	return result, nil
}

// AddKey adds a new SSH key to GitLab
func (c *GitLabClient) AddKey(ctx context.Context, title, publicKey string) (string, error) {
	logger.Debug("Adding SSH key to GitLab: %s", title)

	payload := map[string]string{
		"title": title,
		"key":   publicKey,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v4/user/keys", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to add GitLab key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(body))
	}

	var key gitlabKey
	if err := json.NewDecoder(resp.Body).Decode(&key); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	keyID := fmt.Sprintf("%d", key.ID)
	logger.Info("Added SSH key to GitLab: %s (ID: %s)", title, keyID)
	return keyID, nil
}

// DeleteKey removes an SSH key from GitLab
func (c *GitLabClient) DeleteKey(ctx context.Context, keyID string) error {
	logger.Debug("Deleting GitLab SSH key: %s", keyID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+"/api/v4/user/keys/"+keyID, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete GitLab key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(body))
	}

	logger.Info("Deleted SSH key from GitLab: %s", keyID)
	return nil
}

// GetKey retrieves a specific SSH key from GitLab
func (c *GitLabClient) GetKey(ctx context.Context, keyID string) (*SSHKey, error) {
	logger.Debug("Getting GitLab SSH key: %s", keyID)

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v4/user/keys/"+keyID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitLab key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(body))
	}

	var key gitlabKey
	if err := json.NewDecoder(resp.Body).Decode(&key); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	result := &SSHKey{
		ID:        fmt.Sprintf("%d", key.ID),
		Title:     key.Title,
		Key:       key.Key,
		CreatedAt: key.CreatedAt,
	}

	return result, nil
}
