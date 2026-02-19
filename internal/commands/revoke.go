package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kunlu/git-keys/internal/api"
	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
	"github.com/kunlu/git-keys/internal/sshkey"
	"github.com/spf13/cobra"
)

var (
	revokeAll         bool
	revokeLocal       bool
	revokeFingerprint string
	revokePersona     string
	revokePlatform    string
)

var revokeCmd = &cobra.Command{
	Use:   "revoke [persona/platform]",
	Short: "Revoke SSH keys from remote platforms",
	Long: `Remove SSH keys from remote platforms (GitHub/GitLab).

By default, keys are only removed from remote platforms. Use --local to also
delete the local key files.

Examples:
  # Revoke all keys for a specific persona
  git-keys revoke personal

  # Revoke all keys on all platforms
  git-keys revoke --all

  # Revoke a specific key by fingerprint
  git-keys revoke --fingerprint SHA256:abc123...

  # Revoke and delete local files
  git-keys revoke personal --local
`,
	RunE: runRevoke,
}

func init() {
	revokeCmd.Flags().BoolVar(&revokeAll, "all", false, "Revoke all keys")
	revokeCmd.Flags().BoolVar(&revokeLocal, "local", false, "Also delete local key files")
	revokeCmd.Flags().StringVar(&revokeFingerprint, "fingerprint", "", "Revoke specific key by fingerprint")
	revokeCmd.Flags().StringVar(&revokePersona, "persona", "", "Revoke keys for specific persona")
	revokeCmd.Flags().StringVar(&revokePlatform, "platform", "", "Revoke keys for specific platform (github/gitlab)")
	rootCmd.AddCommand(revokeCmd)
}

func runRevoke(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load configuration
	configPath := cfgFile
	if configPath == "" {
		configPath = config.GetDefaultConfigPath()
	}

	mgr := config.NewManager(configPath)
	if !mgr.Exists() {
		return fmt.Errorf("configuration file not found at %s\nRun 'git-keys init' first", configPath)
	}

	cfg, err := mgr.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Determine what to revoke
	var targetPersona string
	var targetPlatform string

	if len(args) > 0 {
		// Parse persona/platform from argument
		parts := strings.Split(args[0], "/")
		targetPersona = parts[0]
		if len(parts) > 1 {
			targetPlatform = parts[1]
		}
	} else if revokePersona != "" {
		targetPersona = revokePersona
		targetPlatform = revokePlatform
	} else if revokeFingerprint != "" {
		return revokeByFingerprint(ctx, cfg, revokeFingerprint)
	} else if !revokeAll {
		return fmt.Errorf("specify a persona, use --all, or use --fingerprint")
	}

	// Collect keys to revoke
	var keysToRevoke []keyRevocation

	for _, persona := range cfg.Personas {
		if targetPersona != "" && persona.Name != targetPersona {
			continue
		}

		for _, platform := range persona.Platforms {
			if targetPlatform != "" && string(platform.Type) != targetPlatform {
				continue
			}

			for _, key := range platform.Keys {
				if key.Status == config.KeyStatusRevoked {
					logger.Debug("Key already revoked: %s", key.Fingerprint)
					continue
				}

				keysToRevoke = append(keysToRevoke, keyRevocation{
					Persona:     persona.Name,
					Platform:    platform.Type,
					Account:     platform.Account,
					BaseURL:     platform.BaseURL,
					Key:         key,
					PersonaRef:  &persona,
					PlatformRef: &platform,
				})
			}
		}
	}

	if len(keysToRevoke) == 0 {
		fmt.Println("No keys to revoke.")
		return nil
	}

	// Show what will be revoked
	fmt.Println("\n🔑 Keys to Revoke:")
	fmt.Println("==================")
	for _, kr := range keysToRevoke {
		fmt.Printf("\n  Persona: %s\n", kr.Persona)
		fmt.Printf("  Platform: %s (%s)\n", kr.Platform, kr.Account)
		fmt.Printf("  Fingerprint: %s\n", kr.Key.Fingerprint)
		fmt.Printf("  Local Path: %s\n", kr.Key.LocalPath)
		if kr.Key.RemoteID != "" {
			fmt.Printf("  Remote ID: %s\n", kr.Key.RemoteID)
		}
	}
	fmt.Println()

	// Confirm
	fmt.Print("Revoke these keys from remote platforms? (y/n): ")
	var response string
	fmt.Scanln(&response)
	if strings.ToLower(response) != "y" {
		fmt.Println("Revocation cancelled.")
		return nil
	}

	// Revoke keys
	fmt.Println("\n⚙️  Revoking keys...")
	for i := range keysToRevoke {
		kr := &keysToRevoke[i]
		if err := revokeKey(ctx, kr); err != nil {
			logger.Error("Failed to revoke %s/%s: %v", kr.Persona, kr.Platform, err)
			fmt.Printf("  ❌ %s/%s: %v\n", kr.Persona, kr.Platform, err)
			continue
		}
		fmt.Printf("  ✓ Revoked %s/%s from remote\n", kr.Persona, kr.Platform)

		// Update key status in config
		kr.Key.Status = config.KeyStatusRevoked
	}

	// Delete local files if requested
	if revokeLocal {
		fmt.Println("\n🗑️  Deleting local key files...")
		sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
		keyMgr := sshkey.NewManager(sshDir)

		for _, kr := range keysToRevoke {
			if kr.Key.LocalPath == "" {
				continue
			}

			if err := keyMgr.DeleteKey(kr.Key.LocalPath); err != nil {
				logger.Warn("Failed to delete local key %s: %v", kr.Key.LocalPath, err)
				fmt.Printf("  ⚠️  %s: %v\n", kr.Key.LocalPath, err)
			} else {
				fmt.Printf("  ✓ Deleted %s\n", kr.Key.LocalPath)
			}
		}
	}

	// Save updated configuration
	if err := mgr.Save(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println("\n✅ Revocation complete!")
	if !revokeLocal {
		fmt.Println("\nLocal key files were not deleted (use --local to delete them)")
	}

	return nil
}

type keyRevocation struct {
	Persona     string
	Platform    config.PlatformType
	Account     string
	BaseURL     string
	Key         config.KeyConfig
	PersonaRef  *config.Persona
	PlatformRef *config.Platform
}

func revokeKey(ctx context.Context, kr *keyRevocation) error {
	if kr.Key.RemoteID == "" {
		logger.Debug("No remote ID for key, skipping remote revocation")
		return nil
	}

	// Get API token
	var tokenService string
	if kr.Platform == config.PlatformGitHub {
		tokenService = "git-keys-github"
	} else if kr.Platform == config.PlatformGitLab {
		tokenService = "git-keys-gitlab"
	} else {
		return fmt.Errorf("unsupported platform: %s", kr.Platform)
	}

	tokenMgr := api.NewTokenManager(tokenService)
	token, err := tokenMgr.GetToken(kr.Account)
	if err != nil {
		// Try default account
		token, err = tokenMgr.GetToken("default")
		if err != nil {
			return fmt.Errorf("no API token found (service: %s): %w", tokenService, err)
		}
	}

	// Create API client
	var client api.PlatformClient
	if kr.Platform == config.PlatformGitHub {
		client = api.NewGitHubClient(token)
	} else if kr.Platform == config.PlatformGitLab {
		baseURL := kr.BaseURL
		if baseURL == "" {
			baseURL = "https://gitlab.com"
		}
		client = api.NewGitLabClient(baseURL, token)
	}

	// Delete key from platform
	if err := client.DeleteKey(ctx, kr.Key.RemoteID); err != nil {
		return fmt.Errorf("failed to delete key from platform: %w", err)
	}

	return nil
}

func revokeByFingerprint(ctx context.Context, cfg *config.Config, fingerprint string) error {
	// Normalize fingerprint (strip SHA256: prefix if present)
	fingerprint = strings.TrimPrefix(fingerprint, "SHA256:")

	var found *keyRevocation

	for _, persona := range cfg.Personas {
		for _, platform := range persona.Platforms {
			for _, key := range platform.Keys {
				keyFP := strings.TrimPrefix(key.Fingerprint, "SHA256:")
				if keyFP == fingerprint {
					found = &keyRevocation{
						Persona:     persona.Name,
						Platform:    platform.Type,
						Account:     platform.Account,
						BaseURL:     platform.BaseURL,
						Key:         key,
						PersonaRef:  &persona,
						PlatformRef: &platform,
					}
					break
				}
			}
			if found != nil {
				break
			}
		}
		if found != nil {
			break
		}
	}

	if found == nil {
		return fmt.Errorf("no key found with fingerprint: %s", fingerprint)
	}

	fmt.Printf("\nFound key:\n")
	fmt.Printf("  Persona: %s\n", found.Persona)
	fmt.Printf("  Platform: %s\n", found.Platform)
	fmt.Printf("  Fingerprint: %s\n", found.Key.Fingerprint)
	fmt.Println()

	fmt.Print("Revoke this key? (y/n): ")
	var response string
	fmt.Scanln(&response)
	if strings.ToLower(response) != "y" {
		fmt.Println("Revocation cancelled.")
		return nil
	}

	// Revoke from remote platform
	if err := revokeKey(ctx, found); err != nil {
		return fmt.Errorf("failed to revoke key: %w", err)
	}

	// Update key status in config
	found.Key.Status = config.KeyStatusRevoked
	for i := range found.PlatformRef.Keys {
		if found.PlatformRef.Keys[i].Fingerprint == found.Key.Fingerprint {
			found.PlatformRef.Keys[i].Status = config.KeyStatusRevoked
			break
		}
	}

	// Save configuration
	mgr := config.NewManager(config.GetDefaultConfigPath())
	if err := mgr.Save(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println("\n✅ Key revoked successfully!")
	return nil
}
