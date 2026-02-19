package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
	"github.com/kunlu/git-keys/internal/platform"
	"github.com/kunlu/git-keys/internal/sshconfig"
	"github.com/kunlu/git-keys/internal/sshkey"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply the configuration changes",
	Long:  `Generate SSH keys and update configuration.`,
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

	fmt.Println("\n✅ Successfully applied configuration!")
	fmt.Println("\nYour SSH keys are ready.")
	fmt.Printf("\nSSH config: %s\n", cfg.Defaults.SSHConfigPath)
	fmt.Println("\nNote: You'll need to manually upload the keys to GitHub/GitLab")
	fmt.Println("Use: cat ~/.ssh/<keyfile>.pub and add it to your platform's SSH keys")

	return nil
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
	entries := []sshconfig.Entry{
		{
			Host:         fmt.Sprintf("%s.%s", hostname, persona.Name),
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
