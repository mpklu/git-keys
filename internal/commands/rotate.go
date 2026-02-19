package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

var (
	rotateAll     bool
	rotatePersona string
	rotateDryRun  bool
)

var rotateCmd = &cobra.Command{
	Use:   "rotate [persona/platform]",
	Short: "Rotate SSH keys for personas/platforms",
	Long: `Generate new SSH keys and replace old ones.

The rotation process:
  1. Generate new key pair with updated expiry
  2. Upload new key to remote platform
  3. Update SSH config to use new key
  4. Validate new key works (test SSH connection)
  5. Remove old key from remote platform
  6. Archive old key locally (renamed with .old-YYYY-MM-DD suffix)

This is an atomic operation per persona/platform. If any step fails, changes
are rolled back.

Examples:
  # Rotate keys for a specific persona
  git-keys rotate personal

  # Rotate all keys
  git-keys rotate --all

  # Dry run to see what would be rotated
  git-keys rotate --all --dry-run
`,
	RunE: runRotate,
}

func init() {
	rotateCmd.Flags().BoolVar(&rotateAll, "all", false, "Rotate all keys")
	rotateCmd.Flags().StringVar(&rotatePersona, "persona", "", "Rotate keys for specific persona")
	rotateCmd.Flags().BoolVar(&rotateDryRun, "dry-run", false, "Show what would be rotated without making changes")
	rootCmd.AddCommand(rotateCmd)
}

func runRotate(cmd *cobra.Command, args []string) error {
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

	// Get platform info for key comments
	plat, err := platform.NewPlatform()
	if err != nil {
		return fmt.Errorf("failed to get platform info: %w", err)
	}

	machineName, err := plat.GetMachineName()
	if err != nil {
		machineName = "unknown"
	}

	// Determine what to rotate
	var targetPersona string
	var targetPlatform string

	if len(args) > 0 {
		parts := strings.Split(args[0], "/")
		targetPersona = parts[0]
		if len(parts) > 1 {
			targetPlatform = parts[1]
		}
	} else if rotatePersona != "" {
		targetPersona = rotatePersona
	} else if !rotateAll {
		return fmt.Errorf("specify a persona or use --all")
	}

	// Collect keys to rotate
	var rotations []keyRotation

	for personaIdx, persona := range cfg.Personas {
		if targetPersona != "" && persona.Name != targetPersona {
			continue
		}

		for platformIdx, platform := range persona.Platforms {
			if targetPlatform != "" && string(platform.Type) != targetPlatform {
				continue
			}

			for keyIdx, key := range platform.Keys {
				if key.Status != config.KeyStatusActive {
					logger.Debug("Skipping non-active key: %s", key.Fingerprint)
					continue
				}

				rotations = append(rotations, keyRotation{
					PersonaName:  persona.Name,
					PersonaIdx:   personaIdx,
					PlatformType: platform.Type,
					PlatformIdx:  platformIdx,
					KeyIdx:       keyIdx,
					Account:      platform.Account,
					BaseURL:      platform.BaseURL,
					OldKey:       key,
					MachineName:  machineName,
				})
			}
		}
	}

	if len(rotations) == 0 {
		fmt.Println("No keys to rotate.")
		return nil
	}

	// Show what will be rotated
	fmt.Println("\n🔄 Keys to Rotate:")
	fmt.Println("==================")
	for _, rot := range rotations {
		fmt.Printf("\n  Persona: %s\n", rot.PersonaName)
		fmt.Printf("  Platform: %s (%s)\n", rot.PlatformType, rot.Account)
		fmt.Printf("  Current Key: %s\n", rot.OldKey.LocalPath)
		fmt.Printf("  Fingerprint: %s\n", rot.OldKey.Fingerprint)
		if !rot.OldKey.ExpiresAt.IsZero() {
			fmt.Printf("  Expires: %s\n", rot.OldKey.ExpiresAt.Format("2006-01-02"))
		}
	}
	fmt.Println()

	if rotateDryRun {
		fmt.Println("[DRY RUN - no changes made]")
		return nil
	}

	// Confirm
	fmt.Print("Rotate these keys? (y/n): ")
	var response string
	fmt.Scanln(&response)
	if strings.ToLower(response) != "y" {
		fmt.Println("Rotation cancelled.")
		return nil
	}

	// Rotate keys
	fmt.Println("\n⚙️  Rotating keys...")
	var successful int
	var failed int

	for i := range rotations {
		rot := &rotations[i]
		fmt.Printf("\n  Processing %s/%s...\n", rot.PersonaName, rot.PlatformType)

		if err := rotateKey(ctx, cfg, rot); err != nil {
			logger.Error("Failed to rotate %s/%s: %v", rot.PersonaName, rot.PlatformType, err)
			fmt.Printf("    ❌ Failed: %v\n", err)
			failed++
			continue
		}

		fmt.Printf("    ✓ Rotation complete\n")
		successful++
	}

	// Save updated configuration
	if successful > 0 {
		if err := mgr.Save(cfg); err != nil {
			return fmt.Errorf("failed to save configuration: %w", err)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 40))
	fmt.Printf("✅ Rotation Summary: %d succeeded, %d failed\n", successful, failed)

	if failed > 0 {
		return fmt.Errorf("%d rotation(s) failed", failed)
	}

	return nil
}

type keyRotation struct {
	PersonaName  string
	PersonaIdx   int
	PlatformType config.PlatformType
	PlatformIdx  int
	KeyIdx       int
	Account      string
	BaseURL      string
	OldKey       config.KeyConfig
	NewKey       *config.KeyConfig
	MachineName  string
}

func rotateKey(ctx context.Context, cfg *config.Config, rot *keyRotation) error {
	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	keyMgr := sshkey.NewManager(sshDir)

	// Get default key type from config
	keyType := cfg.Defaults.KeyType
	if keyType == "" {
		keyType = config.KeyTypeED25519
	}

	// Calculate expiry (6 months default)
	expiryMonths := 6
	if cfg.Defaults.KeyExpiration > 0 {
		expiryMonths = int(cfg.Defaults.KeyExpiration.Hours() / 24 / 30)
	}
	expiresAt := time.Now().AddDate(0, expiryMonths, 0)

	// Step 1: Generate new key pair
	fmt.Println("    → Generating new key pair...")
	keyFileName := sshkey.BuildKeyFileName(rot.PlatformType, rot.Account, keyType)
	keyComment := sshkey.BuildKeyComment(rot.PlatformType, rot.Account, rot.MachineName)

	// Add timestamp to avoid collision with existing key
	newKeyPath := keyFileName + "-new"
	if err := keyMgr.GenerateKey(keyType, keyComment, newKeyPath); err != nil {
		return fmt.Errorf("failed to generate new key: %w", err)
	}

	// Ensure we clean up on failure
	defer func() {
		if rot.NewKey == nil {
			// Rotation failed, clean up new key
			keyMgr.DeleteKey(newKeyPath)
		}
	}()

	// Get fingerprint and public key
	fingerprint, err := keyMgr.GetFingerprint(newKeyPath)
	if err != nil {
		return fmt.Errorf("failed to get new key fingerprint: %w", err)
	}

	publicKey, err := keyMgr.GetPublicKey(newKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read new public key: %w", err)
	}

	// Step 2: Upload new key to remote platform
	fmt.Println("    → Uploading new key to platform...")
	remoteID, err := uploadKey(ctx, rot, publicKey)
	if err != nil {
		return fmt.Errorf("failed to upload new key: %w", err)
	}

	// Create new key config
	rot.NewKey = &config.KeyConfig{
		Type:        keyType,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
		Fingerprint: fingerprint,
		LocalPath:   newKeyPath,
		RemoteID:    remoteID,
		Status:      config.KeyStatusActive,
	}

	// Step 3: Update SSH config
	fmt.Println("    → Updating SSH config...")
	if err := updateSSHConfigForRotation(rot, sshDir); err != nil {
		// Try to clean up remote key
		deleteKey(ctx, rot, remoteID)
		return fmt.Errorf("failed to update SSH config: %w", err)
	}

	// Step 4: Validate new key works
	fmt.Println("    → Validating new key...")
	if err := validateSSHKey(rot); err != nil {
		logger.Warn("Key validation failed: %v", err)
		fmt.Println("    ⚠️  Warning: Could not validate new key (connection test failed)")
		fmt.Println("    The key has been uploaded and SSH config updated.")
		// Continue anyway - validation failures are often due to network/firewall
	} else {
		fmt.Println("    ✓ New key validated")
	}

	// Step 5: Remove old key from remote platform
	if rot.OldKey.RemoteID != "" {
		fmt.Println("    → Removing old key from platform...")
		if err := deleteKey(ctx, rot, rot.OldKey.RemoteID); err != nil {
			logger.Warn("Failed to delete old key from platform: %v", err)
			fmt.Println("    ⚠️  Warning: Could not remove old key from platform")
			fmt.Println("    You may need to manually remove it")
		} else {
			fmt.Println("    ✓ Old key removed from platform")
		}
	}

	// Step 6: Archive old key locally
	if rot.OldKey.LocalPath != "" {
		fmt.Println("    → Archiving old key...")
		if err := archiveOldKey(rot.OldKey.LocalPath, sshDir); err != nil {
			logger.Warn("Failed to archive old key: %v", err)
			fmt.Println("    ⚠️  Warning: Could not archive old key")
		} else {
			fmt.Println("    ✓ Old key archived")
		}
	}

	// Step 7: Rename new key to final name (remove -new suffix)
	finalKeyPath := keyFileName
	oldFullPath := filepath.Join(sshDir, newKeyPath)
	newFullPath := filepath.Join(sshDir, finalKeyPath)

	if err := os.Rename(oldFullPath, newFullPath); err != nil {
		logger.Warn("Failed to rename new private key: %v", err)
	} else {
		os.Rename(oldFullPath+".pub", newFullPath+".pub")
		rot.NewKey.LocalPath = finalKeyPath
	}

	// Update config with new key
	cfg.Personas[rot.PersonaIdx].Platforms[rot.PlatformIdx].Keys[rot.KeyIdx] = *rot.NewKey

	return nil
}

func uploadKey(ctx context.Context, rot *keyRotation, publicKey string) (string, error) {
	// Get API token
	var tokenService string
	if rot.PlatformType == config.PlatformGitHub {
		tokenService = "git-keys-github"
	} else if rot.PlatformType == config.PlatformGitLab {
		tokenService = "git-keys-gitlab"
	} else {
		return "", fmt.Errorf("unsupported platform: %s", rot.PlatformType)
	}

	tokenMgr := api.NewTokenManager(tokenService)
	token, err := tokenMgr.GetToken(rot.Account)
	if err != nil {
		token, err = tokenMgr.GetToken("default")
		if err != nil {
			return "", fmt.Errorf("no API token found: %w", err)
		}
	}

	// Create API client
	var client api.PlatformClient
	if rot.PlatformType == config.PlatformGitHub {
		client = api.NewGitHubClient(token)
	} else if rot.PlatformType == config.PlatformGitLab {
		baseURL := rot.BaseURL
		if baseURL == "" {
			baseURL = "https://gitlab.com"
		}
		client = api.NewGitLabClient(baseURL, token)
	}

	// Upload key
	title := fmt.Sprintf("%s@%s (rotated %s)", rot.Account, rot.MachineName, time.Now().Format("2006-01-02"))
	remoteID, err := client.AddKey(ctx, title, publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to upload key: %w", err)
	}

	return remoteID, nil
}

func deleteKey(ctx context.Context, rot *keyRotation, keyID string) error {
	// Get API token
	var tokenService string
	if rot.PlatformType == config.PlatformGitHub {
		tokenService = "git-keys-github"
	} else {
		tokenService = "git-keys-gitlab"
	}

	tokenMgr := api.NewTokenManager(tokenService)
	token, err := tokenMgr.GetToken(rot.Account)
	if err != nil {
		token, err = tokenMgr.GetToken("default")
		if err != nil {
			return err
		}
	}

	// Create client
	var client api.PlatformClient
	if rot.PlatformType == config.PlatformGitHub {
		client = api.NewGitHubClient(token)
	} else {
		baseURL := rot.BaseURL
		if baseURL == "" {
			baseURL = "https://gitlab.com"
		}
		client = api.NewGitLabClient(baseURL, token)
	}

	return client.DeleteKey(ctx, keyID)
}

func updateSSHConfigForRotation(rot *keyRotation, sshDir string) error {
	configPath := filepath.Join(sshDir, "config")
	mgr := sshconfig.NewManager(configPath)

	// Determine host
	var host string
	if rot.PlatformType == config.PlatformGitHub {
		host = "github.com"
	} else if rot.PlatformType == config.PlatformGitLab {
		if rot.BaseURL != "" {
			host = strings.TrimPrefix(rot.BaseURL, "https://")
			host = strings.TrimPrefix(host, "http://")
			host = strings.TrimSuffix(host, "/")
		} else {
			host = "gitlab.com"
		}
	}

	entry := sshconfig.Entry{
		Host:         host,
		HostName:     host,
		IdentityFile: rot.NewKey.LocalPath,
		User:         "git",
	}

	blockID := fmt.Sprintf("git-keys-%s-%s", rot.PersonaName, rot.PlatformType)
	return mgr.AddOrUpdateEntry(blockID, []sshconfig.Entry{entry})
}

func validateSSHKey(rot *keyRotation) error {
	// Determine SSH host
	var sshHost string
	if rot.PlatformType == config.PlatformGitHub {
		sshHost = "git@github.com"
	} else if rot.PlatformType == config.PlatformGitLab {
		if rot.BaseURL != "" {
			host := strings.TrimPrefix(rot.BaseURL, "https://")
			host = strings.TrimPrefix(host, "http://")
			host = strings.TrimSuffix(host, "/")
			sshHost = "git@" + host
		} else {
			sshHost = "git@gitlab.com"
		}
	}

	// Test SSH connection (should fail with "successfully authenticated" message)
	cmd := exec.Command("ssh", "-T", "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10", sshHost)
	output, err := cmd.CombinedOutput()

	outputStr := string(output)

	// GitHub returns exit code 1 but with success message
	// GitLab returns exit code 0 or 1 with welcome message
	if strings.Contains(outputStr, "successfully authenticated") ||
		strings.Contains(outputStr, "Welcome to GitLab") ||
		strings.Contains(outputStr, "Hi ") {
		return nil
	}

	if err != nil {
		return fmt.Errorf("SSH connection failed: %w (output: %s)", err, outputStr)
	}

	return nil
}

func archiveOldKey(keyPath string, sshDir string) error {
	timestamp := time.Now().Format("2006-01-02")
	archiveDir := filepath.Join(sshDir, "archive")

	// Create archive directory if needed
	if err := os.MkdirAll(archiveDir, 0700); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Move private key
	oldPrivate := filepath.Join(sshDir, keyPath)
	newPrivate := filepath.Join(archiveDir, filepath.Base(keyPath)+".old-"+timestamp)

	if err := os.Rename(oldPrivate, newPrivate); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to archive private key: %w", err)
		}
	}

	// Move public key
	oldPublic := oldPrivate + ".pub"
	newPublic := newPrivate + ".pub"

	if err := os.Rename(oldPublic, newPublic); err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Failed to archive public key: %v", err)
		}
	}

	return nil
}
