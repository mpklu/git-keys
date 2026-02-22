package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kunlu/git-keys/internal/api"
	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
	"github.com/kunlu/git-keys/internal/platform"
	"github.com/kunlu/git-keys/internal/sshconfig"
	"github.com/kunlu/git-keys/internal/sshkey"
	"github.com/spf13/cobra"
)

// sanitizeHostname removes spaces and special characters from a string to make it valid for SSH hostnames
func sanitizeHostname(name string) string {
	// Replace spaces with no separator (or could use dash/underscore)
	sanitized := strings.ReplaceAll(name, " ", "")
	// Remove other potentially problematic characters
	sanitized = strings.ReplaceAll(sanitized, "@", "")
	sanitized = strings.ReplaceAll(sanitized, "#", "")
	sanitized = strings.ReplaceAll(sanitized, "$", "")
	return sanitized
}

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply the configuration changes",
	Long:  `Generate SSH keys, upload to platforms, update SSH config, and configure git identity switching.`,
	RunE:  runApply,
}

var (
	applyYes bool
)

func init() {
	applyCmd.Flags().BoolVarP(&applyYes, "yes", "y", false, "skip confirmation prompts")
	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) error {
	logger.Info("Applying configuration...")

	// Load config
	configPath := cfgFile
	if configPath == "" {
		configPath = config.GetDefaultConfigPath()
	}

	mgr := config.NewManager(configPath)
	if !mgr.Exists() {
		return fmt.Errorf("configuration file not found. Run 'git-keys init' first")
	}

	cfg, err := mgr.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get platform info
	plat, err := platform.NewPlatform()
	if err != nil {
		return fmt.Errorf("failed to initialize platform: %w", err)
	}

	machineName, err := plat.GetMachineName()
	if err != nil {
		machineName = "unknown"
	}

	// Confirm unless -y flag
	if !applyYes {
		fmt.Print("\nThis will generate SSH keys and modify your SSH config. Continue? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Initialize managers
	keyMgr := sshkey.NewManager("")
	sshMgr := sshconfig.NewManager(cfg.Defaults.SSHConfigPath)

	// Backup SSH config
	if _, err := sshMgr.BackupConfig(); err != nil {
		logger.Warn("Failed to backup SSH config: %v", err)
	}

	configChanged := false

	// Process each persona and platform
	for personaIdx := range cfg.Personas {
		persona := &cfg.Personas[personaIdx]

		for platformIdx := range persona.Platforms {
			platform := &persona.Platforms[platformIdx]

			logger.Info("Processing %s/%s for persona %s", platform.Type, platform.Account, persona.Name)

			// Check if active key exists
			activeKey := platform.GetActiveKey()

			if activeKey == nil {
				// Generate new key
				keyFileName := sshkey.BuildKeyFileName(platform.Type, platform.Account, cfg.Defaults.KeyType)
				keyComment := sshkey.BuildKeyComment(platform.Type, platform.Account, machineName)

				logger.Info("Generating new %s key: %s", cfg.Defaults.KeyType, keyFileName)

				if err := keyMgr.GenerateKey(cfg.Defaults.KeyType, keyComment, keyFileName); err != nil {
					return fmt.Errorf("failed to generate key: %w", err)
				}

				// Get fingerprint
				fingerprint, err := keyMgr.GetFingerprint(keyFileName)
				if err != nil {
					return fmt.Errorf("failed to get fingerprint: %w", err)
				}

				// Create key config
				newKey := config.KeyConfig{
					Type:        cfg.Defaults.KeyType,
					CreatedAt:   time.Now(),
					ExpiresAt:   time.Now().AddDate(0, 6, 0), // 6 months default
					Fingerprint: fingerprint,
					LocalPath:   keyFileName,
					Status:      config.KeyStatusActive,
				}

				platform.Keys = append(platform.Keys, newKey)
				activeKey = &platform.Keys[len(platform.Keys)-1]
				configChanged = true

				fmt.Printf("✓ Generated key: %s\n", keyFileName)
			}

			// Update SSH config
			if err := updateSSHConfig(sshMgr, persona, platform, activeKey); err != nil {
				return fmt.Errorf("failed to update SSH config: %w", err)
			}

			fmt.Printf("✓ Updated SSH config for %s@%s\n", platform.Account, platform.Type)
		}
	}

	// Save updated config if changed
	if configChanged {
		if err := mgr.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		logger.Info("Configuration updated")
	}

	// Try to automatically upload keys to platforms
	fmt.Println("\n🔑 Uploading keys to platforms...")
	ctx := context.Background()
	envTokens := loadTokensFromEnv()

	for personaIdx := range cfg.Personas {
		persona := &cfg.Personas[personaIdx]
		for platformIdx := range persona.Platforms {
			platform := &persona.Platforms[platformIdx]
			activeKey := platform.GetActiveKey()

			if activeKey == nil || activeKey.RemoteID != "" {
				continue // Skip if no key or already uploaded
			}

			// Try to upload key
			if err := uploadKeyToPlatform(ctx, persona, platform, activeKey, machineName, envTokens); err != nil {
				logger.Warn("Failed to upload key for %s/%s: %v", persona.Name, platform.Type, err)
				fmt.Printf("⚠️  Could not auto-upload key for %s@%s: %v\n", platform.Account, platform.Type, err)
				fmt.Printf("   Please upload manually: cat ~/.ssh/%s.pub\n", activeKey.LocalPath)
			} else {
				configChanged = true
				fmt.Printf("✓ Uploaded key to %s@%s\n", platform.Account, platform.Type)
			}
		}
	}

	// Save config again if keys were uploaded
	if configChanged {
		if err := mgr.Save(cfg); err != nil {
			logger.Warn("Failed to save config after upload: %v", err)
		}
	}

	// Setup git configuration for personas
	fmt.Println("\n⚙️  Setting up git configuration...")
	if err := setupGitConfigForPersonas(cfg, &configChanged); err != nil {
		logger.Warn("Failed to setup git config: %v", err)
		fmt.Printf("⚠️  Git config setup had issues. You can run 'git-keys setup-git' manually.\n")
	}

	// Save config if gitdir was added
	if configChanged {
		if err := mgr.Save(cfg); err != nil {
			logger.Warn("Failed to save config after git setup: %v", err)
		}
	}

	fmt.Println("\n✅ Successfully applied configuration!")
	fmt.Println("\nYour SSH keys are ready.")
	fmt.Printf("\nSSH config: %s\n", cfg.Defaults.SSHConfigPath)

	return nil
}

// loadTokensFromEnv reads API tokens from .env file in current directory
func loadTokensFromEnv() map[string]string {
	tokens := make(map[string]string)

	// Try to read .env file from current directory
	envPath := ".env"
	data, err := os.ReadFile(envPath)
	if err != nil {
		// Also try from project root
		if cwd, err := os.Getwd(); err == nil {
			envPath = filepath.Join(cwd, ".env")
			data, err = os.ReadFile(envPath)
		}
		if err != nil {
			return tokens
		}
	}

	// Parse .env file
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			tokens[key] = value
		}
	}

	return tokens
}

// getTokenForPlatform retrieves token from env map or prompts user
func getTokenForPlatform(platformType config.PlatformType, account string, envTokens map[string]string) (string, error) {
	var tokenKey string

	if platformType == config.PlatformGitHub {
		tokenKey = fmt.Sprintf("GITHUB_API_TOKEN_%s", account)
	} else if platformType == config.PlatformGitLab {
		tokenKey = fmt.Sprintf("GITLAB_TOKEN_%s", account)
	} else {
		return "", fmt.Errorf("unsupported platform: %s", platformType)
	}

	// Check if token exists in env
	if token, ok := envTokens[tokenKey]; ok && token != "" {
		return token, nil
	}

	// Prompt user for token
	fmt.Printf("\n🔑 API token for %s@%s not found in .env\n", account, platformType)
	fmt.Printf("   Expected: %s=<token>\n", tokenKey)
	fmt.Print("   Enter token now (or press Enter to skip): ")

	reader := bufio.NewReader(os.Stdin)
	token, _ := reader.ReadString('\n')
	token = strings.TrimSpace(token)

	if token == "" {
		return "", fmt.Errorf("no token provided")
	}

	return token, nil
}

// uploadKeyToPlatform uploads SSH key to GitHub/GitLab
func uploadKeyToPlatform(ctx context.Context, persona *config.Persona, platform *config.Platform, key *config.KeyConfig, machineName string, envTokens map[string]string) error {
	// Get API token
	token, err := getTokenForPlatform(platform.Type, platform.Account, envTokens)
	if err != nil {
		return err
	}

	// Read public key
	pubKeyPath := filepath.Join(os.Getenv("HOME"), ".ssh", key.LocalPath+".pub")
	pubKeyData, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}
	publicKey := strings.TrimSpace(string(pubKeyData))

	// Create API client
	var client api.PlatformClient
	if platform.Type == config.PlatformGitHub {
		client = api.NewGitHubClient(token)
	} else if platform.Type == config.PlatformGitLab {
		baseURL := platform.BaseURL
		if baseURL == "" {
			baseURL = "https://gitlab.com"
		}
		client = api.NewGitLabClient(baseURL, token)
	} else {
		return fmt.Errorf("unsupported platform: %s", platform.Type)
	}

	// Upload key
	title := fmt.Sprintf("%s@%s (git-keys %s)", platform.Account, machineName, time.Now().Format("2006-01-02"))
	remoteID, err := client.AddKey(ctx, title, publicKey)
	if err != nil {
		return fmt.Errorf("API error: %w", err)
	}

	// Update key config with remote ID
	key.RemoteID = remoteID

	return nil
}

// setupGitConfigForPersonas creates git config files and includeIf entries
func setupGitConfigForPersonas(cfg *config.Config, configChanged *bool) error {
	reader := bufio.NewReader(os.Stdin)
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	globalGitConfig := filepath.Join(home, ".gitconfig")
	var includeEntries []string
	needsGitConfigUpdate := false

	// Iterate through personas and their platforms
	for personaIdx := range cfg.Personas {
		persona := &cfg.Personas[personaIdx]

		for platformIdx := range persona.Platforms {
			platform := &persona.Platforms[platformIdx]

			// Create a unique identifier for this platform
			platformID := fmt.Sprintf("%s-%s", string(platform.Type), platform.Account)

			// Check if gitdir already configured for this platform
			if platform.GitDir != "" {
				// Create git config file for this persona-platform combo
				configName := fmt.Sprintf(".gitconfig-%s-%s", persona.Name, platformID)
				configPath := filepath.Join(home, configName)

				if err := createPlatformGitConfigFile(persona, platform, configPath); err != nil {
					logger.Warn("Failed to create git config for %s/%s: %v", persona.Name, platformID, err)
					continue
				}

				includeEntry := fmt.Sprintf("[includeIf \"gitdir:%s\"]\n\tpath = %s\n", platform.GitDir, configPath)
				includeEntries = append(includeEntries, includeEntry)
				continue
			}

			// Prompt for gitdir
			fmt.Printf("\n📁 Directory pattern for %s <%s> - %s/%s\n",
				persona.Name, persona.Email, platform.Type, platform.Account)
			fmt.Printf("   This sets where git will use this identity and SSH key\n")
			fmt.Printf("   Example: ~/Projects/%s/\n", platform.Account)
			fmt.Print("   Enter directory pattern (or press Enter to skip): ")

			pattern, _ := reader.ReadString('\n')
			pattern = strings.TrimSpace(pattern)

			if pattern == "" {
				fmt.Printf("   ⚠️  Skipped git config for %s/%s\n", persona.Name, platformID)
				continue
			}

			// Expand ~ to home directory
			if strings.HasPrefix(pattern, "~/") {
				pattern = filepath.Join(home, pattern[2:])
			}

			// Ensure pattern ends with /
			if !strings.HasSuffix(pattern, "/") {
				pattern = pattern + "/"
			}

			platform.GitDir = pattern
			*configChanged = true
			needsGitConfigUpdate = true

			// Create git config file
			configName := fmt.Sprintf(".gitconfig-%s-%s", persona.Name, platformID)
			configPath := filepath.Join(home, configName)

			if err := createPlatformGitConfigFile(persona, platform, configPath); err != nil {
				logger.Warn("Failed to create git config for %s/%s: %v", persona.Name, platformID, err)
				continue
			}

			fmt.Printf("   ✓ Created: %s\n", configPath)

			includeEntry := fmt.Sprintf("[includeIf \"gitdir:%s\"]\n\tpath = %s\n", platform.GitDir, configPath)
			includeEntries = append(includeEntries, includeEntry)
		}
	}

	// Update global gitconfig if needed
	if len(includeEntries) > 0 {
		// Backup first
		if _, err := os.Stat(globalGitConfig); err == nil && needsGitConfigUpdate {
			backupPath := globalGitConfig + ".backup-git-keys"
			content, err := os.ReadFile(globalGitConfig)
			if err == nil {
				os.WriteFile(backupPath, content, 0644)
				fmt.Printf("\n💾 Backed up ~/.gitconfig to ~/.gitconfig.backup-git-keys\n")
			}
		}

		if err := addGitConfigIncludes(globalGitConfig, includeEntries); err != nil {
			return fmt.Errorf("failed to update ~/.gitconfig: %w", err)
		}

		if needsGitConfigUpdate {
			fmt.Printf("✓ Updated ~/.gitconfig with platform configurations\n")
		}
	}

	return nil
}

// createPlatformGitConfigFile creates a git config file for a persona-platform combination
func createPlatformGitConfigFile(persona *config.Persona, platform *config.Platform, configPath string) error {
	var content strings.Builder

	content.WriteString(fmt.Sprintf("# Git configuration for %s <%s>\n", persona.Name, persona.Email))
	content.WriteString(fmt.Sprintf("# Platform: %s/%s\n", platform.Type, platform.Account))
	content.WriteString("# Managed by git-keys\n\n")

	// User identity (from persona)
	content.WriteString("[user]\n")
	content.WriteString(fmt.Sprintf("\tname = %s\n", persona.Name))
	content.WriteString(fmt.Sprintf("\temail = %s\n\n", persona.Email))

	// URL rewrites for this specific platform's SSH host
	var baseHost string

	if platform.Type == config.PlatformGitHub {
		baseHost = "github.com"
	} else if platform.Type == config.PlatformGitLab {
		if platform.BaseURL != "" && platform.BaseURL != "https://gitlab.com" {
			baseHost = strings.TrimPrefix(platform.BaseURL, "https://")
			baseHost = strings.TrimPrefix(baseHost, "http://")
			baseHost = strings.TrimSuffix(baseHost, "/")
		} else {
			baseHost = "gitlab.com"
		}
	}

	if baseHost != "" {
		// Use platform-specific SSH host (e.g., github.com.personal)
		// Sanitize persona name to ensure valid hostname (no spaces)
		sanitizedPersona := sanitizeHostname(persona.Name)
		personaHost := fmt.Sprintf("%s.%s", baseHost, sanitizedPersona)
		content.WriteString("# SSH host rewrite for platform-specific key\n")
		content.WriteString(fmt.Sprintf("[url \"git@%s:\"]\n", personaHost))
		content.WriteString(fmt.Sprintf("\tinsteadOf = git@%s:\n", baseHost))
		content.WriteString(fmt.Sprintf("\tinsteadOf = https://%s/\n\n", baseHost))
	}

	return os.WriteFile(configPath, []byte(content.String()), 0644)
}

// addGitConfigIncludes adds or updates includeIf entries in ~/.gitconfig
func addGitConfigIncludes(gitConfigPath string, entries []string) error {
	// Read existing gitconfig
	var existingContent string
	if data, err := os.ReadFile(gitConfigPath); err == nil {
		existingContent = string(data)
	}

	managedMarker := "# BEGIN git-keys managed conditional includes"
	endMarker := "# END git-keys managed conditional includes"

	var newContent string

	if strings.Contains(existingContent, managedMarker) {
		// Replace existing managed section
		startIdx := strings.Index(existingContent, managedMarker)
		endIdx := strings.Index(existingContent, endMarker)

		if endIdx > startIdx {
			before := existingContent[:startIdx]
			after := existingContent[endIdx+len(endMarker):]

			newContent = strings.TrimRight(before, "\n") + "\n\n" +
				managedMarker + "\n" +
				strings.Join(entries, "\n") +
				endMarker + "\n" +
				strings.TrimLeft(after, "\n")
		}
	} else {
		// Add new managed section at the end
		newContent = strings.TrimRight(existingContent, "\n") + "\n\n" +
			managedMarker + "\n" +
			strings.Join(entries, "\n") +
			endMarker + "\n"
	}

	return os.WriteFile(gitConfigPath, []byte(newContent), 0644)
}

func updateSSHConfig(sshMgr *sshconfig.Manager, persona *config.Persona, platform *config.Platform, key *config.KeyConfig) error {
	logger.Info("Updating SSH config for %s/%s", platform.Type, platform.Account)

	blockID := sshconfig.GetManagedBlockID(persona.Name, platform.Type, platform.Account)

	// Determine hostname based on platform
	hostname := "github.com"
	if platform.Type == config.PlatformGitLab {
		if platform.BaseURL != "" && platform.BaseURL != "https://gitlab.com" {
			hostname = strings.TrimPrefix(platform.BaseURL, "https://")
			hostname = strings.TrimPrefix(hostname, "http://")
		} else {
			hostname = "gitlab.com"
		}
	}

	// Create SSH config entry
	// Sanitize persona name to ensure valid hostname (no spaces)
	sanitizedPersona := sanitizeHostname(persona.Name)
	entries := []sshconfig.Entry{
		{
			Host:         fmt.Sprintf("%s.%s", hostname, sanitizedPersona),
			HostName:     hostname,
			User:         "git",
			IdentityFile: fmt.Sprintf("~/.ssh/%s", key.LocalPath),
			Extra: map[string]string{
				"IdentitiesOnly": "yes",
			},
		},
	}

	if err := sshMgr.AddOrUpdateEntry(blockID, entries); err != nil {
		return fmt.Errorf("failed to update SSH config: %w", err)
	}

	return nil
}
