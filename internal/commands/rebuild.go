package commands

import (
	"bufio"
	"context"
	"encoding/json"
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

const (
	backupTimestampFormat = "2006-01-02-150405"
)

var (
	rebuildInteractive bool
	rebuildKeepRemote  bool
	rebuildSkipBackup  bool
	rebuildDryRun      bool
)

// BackupData represents the backed-up configuration and scan results
type BackupData struct {
	Timestamp      time.Time      `json:"timestamp"`
	OldConfig      *config.Config `json:"old_config,omitempty"`
	ScanResult     *ScanResult    `json:"scan_result"`
	SSHConfigPath  string         `json:"ssh_config_path"`
	RecommendedMap RecommendedMap `json:"recommended_mapping"`
}

// RecommendedMap suggests how to map discovered keys to personas
type RecommendedMap struct {
	Personas []RecommendedPersona `json:"personas"`
}

// RecommendedPersona is a suggested persona based on scan
type RecommendedPersona struct {
	Name      string                `json:"name"`
	Email     string                `json:"email"`
	Platforms []RecommendedPlatform `json:"platforms"`
}

// RecommendedPlatform is a suggested platform configuration
type RecommendedPlatform struct {
	Type    config.PlatformType `json:"type"`
	Account string              `json:"account"`
	BaseURL string              `json:"base_url,omitempty"`
	KeyPath string              `json:"key_path,omitempty"`
}

var rebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild git-keys from scratch with intelligent backup",
	Long: `Scan your current setup, create backups, clean everything, and optionally
rebuild interactively using the backed-up configuration as a guide.

This command is useful when you want to start fresh but don't want to lose
track of your current setup. It's safer than manually deleting everything
because it preserves a record of what you had.

The process:
  1. 🔍 Scan current SSH keys, config, and git identities
  2. 💾 Create timestamped backup of everything
  3. 📋 Show summary of what was found
  4. ⚠️  Confirm cleanup operation
  5. 🧹 Clean up (revoke remote keys, remove config blocks, delete files)
  6. 🎯 Interactive re-setup (if --interactive flag used)
  7. ✅ Generate new keys and apply configuration

Backups are saved to ~/.git-keys/backups/backup-YYYY-MM-DD-HHMMSS.json

Examples:
  # Rebuild with interactive guided setup
  git-keys rebuild --interactive

  # Rebuild without re-setup (just clean)
  git-keys rebuild

  # Keep remote keys, only clean local
  git-keys rebuild --keep-remote
`,
	RunE: runRebuild,
}

func init() {
	rebuildCmd.Flags().BoolVarP(&rebuildInteractive, "interactive", "i", false, "Interactive guided setup after cleanup")
	rebuildCmd.Flags().BoolVar(&rebuildKeepRemote, "keep-remote", false, "Don't revoke keys from remote platforms")
	rebuildCmd.Flags().BoolVar(&rebuildSkipBackup, "skip-backup", false, "Skip creating backup (not recommended)")
	rebuildCmd.Flags().BoolVar(&rebuildDryRun, "dry-run", false, "Show what would be cleaned without making changes")
	rootCmd.AddCommand(rebuildCmd)
}

func runRebuild(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	fmt.Println("\n🔄 Git-Keys Rebuild")
	fmt.Println("==================")
	fmt.Println()

	// Step 1: Scan current setup
	fmt.Println("🔍 Step 1: Scanning current setup...")
	scanResult, err := performScan()
	if err != nil {
		logger.Warn("Scan had issues: %v", err)
		fmt.Printf("⚠️  Scan completed with warnings (continuing...)\n\n")
	} else {
		fmt.Printf("✓ Found %d SSH keys, %d SSH config hosts\n\n", len(scanResult.Keys), len(scanResult.SSHConfigHosts))
	}

	// Step 2: Load existing config (if exists)
	configPath := cfgFile
	if configPath == "" {
		configPath = config.GetDefaultConfigPath()
	}

	mgr := config.NewManager(configPath)
	var existingConfig *config.Config
	if mgr.Exists() {
		existingConfig, err = mgr.Load()
		if err != nil {
			logger.Warn("Failed to load existing config: %v", err)
		}
	}

	// Step 3: Create backup
	var backupPath string
	if !rebuildSkipBackup {
		fmt.Println("💾 Step 2: Creating backup...")
		backupPath, err = createBackup(scanResult, existingConfig)
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
		fmt.Printf("✓ Backup saved to: %s\n\n", backupPath)
	} else {
		fmt.Println("⚠️  Skipping backup (--skip-backup flag)")
		fmt.Println()
	}

	// Step 4: Show summary
	fmt.Println("📋 Step 3: Summary of Current Setup")
	fmt.Println("===================================")
	if existingConfig != nil {
		fmt.Printf("\nCurrent Config:\n")
		fmt.Printf("  • Personas: %d\n", len(existingConfig.Personas))
		for _, p := range existingConfig.Personas {
			fmt.Printf("    - %s (%s) - %d platforms\n", p.Name, p.Email, len(p.Platforms))
		}
	}

	if len(scanResult.Keys) > 0 {
		fmt.Printf("\nDiscovered SSH Keys:\n")
		for i, key := range scanResult.Keys {
			if i >= 5 {
				fmt.Printf("  ... and %d more\n", len(scanResult.Keys)-5)
				break
			}
			fmt.Printf("  • %s (%s)\n", filepath.Base(key.Path), key.Type)
			if key.Comment != "" {
				fmt.Printf("    %s\n", key.Comment)
			}
		}
	}

	if len(scanResult.GitConfig.GlobalEmail) > 0 {
		fmt.Printf("\nGit Identity:\n")
		fmt.Printf("  • %s <%s>\n", scanResult.GitConfig.GlobalName, scanResult.GitConfig.GlobalEmail)
	}

	// Step 5: Confirm cleanup
	fmt.Println("\n⚠️  Step 4: Confirm Cleanup")
	fmt.Println("=========================")
	if rebuildDryRun {
		fmt.Println("\n🔍 DRY RUN MODE - No changes will be made")
	}
	fmt.Println("\nThis will:")
	if !rebuildKeepRemote {
		fmt.Println("  ✓ Revoke keys from remote platforms (GitHub/GitLab)")
	} else {
		fmt.Println("  ○ Keep remote platform keys (--keep-remote)")
	}
	fmt.Println("  ✓ Remove all git-keys managed SSH config blocks")
	fmt.Println("  ✓ Delete git-keys configuration file")
	fmt.Println("  ✓ Clear API tokens from keychain")
	fmt.Println("\nWill NOT:")
	fmt.Println("  ✗ Delete non-git-keys SSH keys")
	fmt.Println("  ✗ Delete entire SSH config (only managed blocks)")

	if !rebuildSkipBackup {
		fmt.Printf("\n💾 Your backup is safe at:\n   %s\n", backupPath)
	}

	if rebuildDryRun {
		fmt.Println("\n✓ Dry run complete. Run without --dry-run to perform cleanup.")
		return nil
	}

	fmt.Print("\nType 'yes' to continue: ")
	var response string
	fmt.Scanln(&response)
	if strings.ToLower(response) != "yes" {
		fmt.Println("\n❌ Rebuild cancelled. No changes made.")
		return nil
	}

	// Step 6: Clean everything
	fmt.Println("\n🧹 Step 5: Cleaning up...")
	if err := performCleanup(ctx, existingConfig, !rebuildKeepRemote); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}
	fmt.Println("✓ Cleanup complete")
	fmt.Println()

	// Step 7: Interactive re-setup
	if rebuildInteractive {
		fmt.Println("🎯 Step 6: Interactive Re-setup")
		fmt.Println("===============================")
		fmt.Println()

		recommended := analyzeAndRecommend(scanResult, existingConfig)
		if err := interactiveRebuild(recommended, scanResult); err != nil {
			return fmt.Errorf("interactive rebuild failed: %w", err)
		}
	} else {
		fmt.Println("✅ Rebuild Complete!")
		fmt.Println("\nNext steps:")
		fmt.Println("  1. Run 'git-keys init' to create new configuration")
		fmt.Println("  2. Run 'git-keys plan' to preview changes")
		fmt.Println("  3. Run 'git-keys apply' to generate new keys")

		if !rebuildSkipBackup {
			fmt.Printf("\nYour old setup is backed up at:\n  %s\n", backupPath)
		}
	}

	return nil
}

func performScan() (*ScanResult, error) {
	// Reuse scan logic from scan command
	result := &ScanResult{}

	sshPath := filepath.Join(os.Getenv("HOME"), ".ssh")

	keys, err := scanSSHKeys(sshPath)
	if err != nil {
		logger.Warn("Failed to scan SSH keys: %v", err)
	} else {
		result.Keys = keys
	}

	hosts, err := scanSSHConfig(sshPath)
	if err != nil {
		logger.Warn("Failed to parse SSH config: %v", err)
	} else {
		result.SSHConfigHosts = hosts
	}

	matchKeysToHosts(result)
	checkSSHAgent(result)

	gitConf, err := scanGitConfig()
	if err != nil {
		logger.Warn("Failed to parse Git config: %v", err)
	} else {
		result.GitConfig = gitConf
	}

	return result, nil
}

func createBackup(scanResult *ScanResult, existingConfig *config.Config) (string, error) {
	timestamp := time.Now()
	backupData := BackupData{
		Timestamp:      timestamp,
		OldConfig:      existingConfig,
		ScanResult:     scanResult,
		SSHConfigPath:  filepath.Join(os.Getenv("HOME"), ".ssh", "config"),
		RecommendedMap: analyzeAndRecommend(scanResult, existingConfig),
	}

	// Create backup directory
	homeDir, _ := os.UserHomeDir()
	backupDir := filepath.Join(homeDir, ".git-keys", "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Create backup file
	backupFilename := fmt.Sprintf("backup-%s.json", timestamp.Format(backupTimestampFormat))
	backupPath := filepath.Join(backupDir, backupFilename)

	data, err := json.MarshalIndent(backupData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal backup data: %w", err)
	}

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	// Also backup SSH config file
	sshConfigPath := filepath.Join(os.Getenv("HOME"), ".ssh", "config")
	if _, err := os.Stat(sshConfigPath); err == nil {
		backupSSHConfig := sshConfigPath + fmt.Sprintf(".pre-rebuild-%s", timestamp.Format(backupTimestampFormat))
		content, err := os.ReadFile(sshConfigPath)
		if err == nil {
			os.WriteFile(backupSSHConfig, content, 0600)
			logger.Info("SSH config backed up to: %s", backupSSHConfig)
		}
	}

	// Backup current config file if exists
	configPath := config.GetDefaultConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		backupConfigPath := configPath + fmt.Sprintf(".pre-rebuild-%s", timestamp.Format(backupTimestampFormat))
		content, err := os.ReadFile(configPath)
		if err == nil {
			os.WriteFile(backupConfigPath, content, 0600)
			logger.Info("Config file backed up to: %s", backupConfigPath)
		}
	}

	return backupPath, nil
}

func analyzeAndRecommend(scanResult *ScanResult, existingConfig *config.Config) RecommendedMap {
	recommended := RecommendedMap{
		Personas: []RecommendedPersona{},
	}

	// If we have existing config, use that as primary source
	if existingConfig != nil {
		for _, persona := range existingConfig.Personas {
			recPersona := RecommendedPersona{
				Name:      persona.Name,
				Email:     persona.Email,
				Platforms: []RecommendedPlatform{},
			}

			for _, platform := range persona.Platforms {
				recPlatform := RecommendedPlatform{
					Type:    platform.Type,
					Account: platform.Account,
					BaseURL: platform.BaseURL,
				}

				// Try to find corresponding key in scan
				if activeKey := platform.GetActiveKey(); activeKey != nil {
					recPlatform.KeyPath = activeKey.LocalPath
				}

				recPersona.Platforms = append(recPersona.Platforms, recPlatform)
			}

			recommended.Personas = append(recommended.Personas, recPersona)
		}
		return recommended
	}

	// Otherwise, infer from scan results
	emailMap := make(map[string]*RecommendedPersona)

	// 1. Create personas from global email
	if scanResult.GitConfig.GlobalEmail != "" {
		persona := &RecommendedPersona{
			Name:      "personal",
			Email:     scanResult.GitConfig.GlobalEmail,
			Platforms: []RecommendedPlatform{},
		}
		emailMap[scanResult.GitConfig.GlobalEmail] = persona
	}

	// 2. Create personas from conditional git configs (this is important!)
	for _, include := range scanResult.GitConfig.Includes {
		if include.Email == "" {
			continue
		}

		// Skip if we already have this email
		if _, exists := emailMap[include.Email]; exists {
			continue
		}

		// Infer persona name from the include name or condition
		personaName := include.Name
		if personaName == "" {
			// Try to extract from condition path
			if strings.Contains(include.Condition, "work") {
				personaName = "work"
			} else if strings.Contains(include.Condition, "personal") {
				personaName = "personal"
			} else {
				// Use the last part of the condition path
				parts := strings.Split(strings.Trim(include.Condition, "~/"), "/")
				if len(parts) > 0 {
					personaName = parts[len(parts)-1]
				} else {
					personaName = include.Name
				}
			}
		}

		persona := &RecommendedPersona{
			Name:      personaName,
			Email:     include.Email,
			Platforms: []RecommendedPlatform{},
		}

		// Add discovered platforms from git repos!
		// Note: Account is empty because we can't determine it from git URLs
		// User will specify their account during interactive setup
		for _, discovered := range include.DiscoveredPlatforms {
			platform := RecommendedPlatform{
				Type:    config.PlatformType(discovered.Type),
				Account: "", // Will be filled in during interactive setup
				BaseURL: discovered.BaseURL,
			}
			persona.Platforms = append(persona.Platforms, platform)
		}

		emailMap[include.Email] = persona
	}

	// 3. Analyze SSH config hosts to infer platforms (if available)
	for _, host := range scanResult.SSHConfigHosts {
		if host.HostName == "github.com" || host.HostName == "gitlab.com" {
			platformType := config.PlatformGitHub
			if strings.Contains(host.HostName, "gitlab") {
				platformType = config.PlatformGitLab
			}

			// Try to infer account from host or identity file
			account := "username"

			// Use first available persona
			var targetPersona *RecommendedPersona
			if scanResult.GitConfig.GlobalEmail != "" {
				targetPersona = emailMap[scanResult.GitConfig.GlobalEmail]
			} else {
				for _, p := range emailMap {
					targetPersona = p
					break
				}
			}

			if targetPersona != nil {
				platform := RecommendedPlatform{
					Type:    platformType,
					Account: account,
					KeyPath: host.IdentityFile,
				}
				targetPersona.Platforms = append(targetPersona.Platforms, platform)
			}
		}
	}

	// Convert map to slice
	for _, persona := range emailMap {
		recommended.Personas = append(recommended.Personas, *persona)
	}

	return recommended
}

func performCleanup(ctx context.Context, existingConfig *config.Config, revokeRemote bool) error {
	// 1. Revoke remote keys if requested
	if revokeRemote && existingConfig != nil {
		fmt.Println("  → Revoking keys from remote platforms...")
		for _, persona := range existingConfig.Personas {
			for _, platform := range persona.Platforms {
				for _, key := range platform.Keys {
					if key.Status != config.KeyStatusActive || key.RemoteID == "" {
						continue
					}

					kr := &keyRevocation{
						Persona:  persona.Name,
						Platform: platform.Type,
						Account:  platform.Account,
						BaseURL:  platform.BaseURL,
						Key:      key,
					}

					if err := revokeKey(ctx, kr); err != nil {
						logger.Warn("Failed to revoke key %s: %v", key.Fingerprint, err)
					} else {
						fmt.Printf("    ✓ Revoked %s/%s\n", persona.Name, platform.Type)
					}
				}
			}
		}
	}

	// 2. Remove all managed blocks from SSH config
	fmt.Println("  → Removing managed SSH config blocks...")
	sshConfigPath := filepath.Join(os.Getenv("HOME"), ".ssh", "config")
	sshMgr := sshconfig.NewManager(sshConfigPath)
	if err := sshMgr.RemoveAllManagedBlocks(); err != nil {
		logger.Warn("Failed to clean SSH config: %v", err)
	} else {
		fmt.Println("    ✓ SSH config cleaned")
	}

	// 3. Delete git-keys managed key files (if tracked in config)
	if existingConfig != nil {
		fmt.Println("  → Deleting git-keys managed key files...")
		sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
		keyMgr := sshkey.NewManager(sshDir)

		deletedCount := 0
		for _, persona := range existingConfig.Personas {
			for _, platform := range persona.Platforms {
				for _, key := range platform.Keys {
					if key.LocalPath == "" {
						continue
					}

					if err := keyMgr.DeleteKey(key.LocalPath); err != nil {
						logger.Warn("Failed to delete key %s: %v", key.LocalPath, err)
					} else {
						deletedCount++
					}
				}
			}
		}
		fmt.Printf("    ✓ Deleted %d key files\n", deletedCount)
	}

	// 4. Delete config file
	fmt.Println("  → Removing configuration file...")
	configPath := config.GetDefaultConfigPath()
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		logger.Warn("Failed to delete config file: %v", err)
	} else {
		fmt.Println("    ✓ Config file removed")
	}

	// 5. Clear keychain tokens
	fmt.Println("  → Clearing API tokens from keychain...")
	tokenServices := []string{"git-keys-github", "git-keys-gitlab"}
	for _, service := range tokenServices {
		tokenMgr := api.NewTokenManager(service)
		// Try to delete tokens for common account names
		accounts := []string{"default", "personal", "work"}
		for _, account := range accounts {
			tokenMgr.DeleteToken(account) // Ignore errors, may not exist
		}
	}
	fmt.Println("    ✓ Tokens cleared")

	return nil
}

func interactiveRebuild(recommended RecommendedMap, scanResult *ScanResult) error {
	plat, err := platform.NewPlatform()
	if err != nil {
		return fmt.Errorf("failed to initialize platform: %w", err)
	}

	machineID, err := plat.GetMachineID()
	if err != nil {
		return fmt.Errorf("failed to get machine ID: %w", err)
	}

	machineName, err := plat.GetMachineName()
	if err != nil {
		machineName = "unknown"
	}

	osVersion, err := plat.GetOSVersion()
	if err != nil {
		osVersion = ""
	}

	// Create new config
	configPath := config.GetDefaultConfigPath()
	mgr := config.NewManager(configPath)
	cfg := mgr.CreateDefault(config.Machine{
		ID:        machineID,
		Name:      machineName,
		OS:        plat.GetOS(),
		OSVersion: osVersion,
	})

	fmt.Println("Based on your backup, I found these identities:")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	for i, recPersona := range recommended.Personas {
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("Identity %d: %s <%s>\n", i+1, recPersona.Name, recPersona.Email)
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		// Show discovered platforms from git repos
		if len(recPersona.Platforms) > 0 {
			fmt.Println("Discovered platforms (from git repos in this directory):")
			for _, p := range recPersona.Platforms {
				if p.Type == "github" {
					fmt.Printf("  ✓ GitHub.com\n")
				} else if p.Type == "gitlab" {
					if p.BaseURL != "" {
						fmt.Printf("  ✓ GitLab (%s)\n", p.BaseURL)
					} else {
						fmt.Printf("  ✓ GitLab.com\n")
					}
				}
			}
			fmt.Println()
		}

		// Ask if user wants to create a persona for this identity
		fmt.Print("Create a persona for this identity? (y/n): ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println()
			continue
		}

		// Allow customizing persona name
		fmt.Printf("Persona name [%s]: ", recPersona.Name)
		personaName, _ := reader.ReadString('\n')
		personaName = strings.TrimSpace(personaName)
		if personaName == "" {
			personaName = recPersona.Name
		}

		persona := config.Persona{
			Name:      personaName,
			Email:     recPersona.Email,
			Platforms: []config.Platform{},
		}

		// Group discovered platforms by type for prompting
		hasGitHub := false
		hasGitLabPublic := false
		gitlabPrivateBaseURLs := []string{}

		for _, p := range recPersona.Platforms {
			if p.Type == "github" && !hasGitHub {
				hasGitHub = true
			} else if p.Type == "gitlab" {
				if p.BaseURL != "" {
					// Check if we already have this baseURL
					found := false
					for _, url := range gitlabPrivateBaseURLs {
						if url == p.BaseURL {
							found = true
							break
						}
					}
					if !found {
						gitlabPrivateBaseURLs = append(gitlabPrivateBaseURLs, p.BaseURL)
					}
				} else if !hasGitLabPublic {
					hasGitLabPublic = true
				}
			}
		}

		// Prompt for GitHub account (if discovered from repos)
		if hasGitHub {
			fmt.Print("\n  Add GitHub account for this persona? (y/n): ")
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response == "y" || response == "yes" {
				fmt.Print("  Enter your GitHub username: ")
				account, _ := reader.ReadString('\n')
				account = strings.TrimSpace(account)

				if account != "" {
					platform := config.Platform{
						Type:    config.PlatformGitHub,
						Account: account,
						Keys:    []config.KeyConfig{},
					}
					persona.Platforms = append(persona.Platforms, platform)
					fmt.Printf("    ✓ Added GitHub account: %s\n", account)
				}
			}
		}

		// Prompt for GitLab.com account (if discovered from repos)
		if hasGitLabPublic {
			fmt.Print("\n  Add GitLab.com account for this persona? (y/n): ")
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response == "y" || response == "yes" {
				fmt.Print("  Enter your GitLab.com username: ")
				account, _ := reader.ReadString('\n')
				account = strings.TrimSpace(account)

				if account != "" {
					platform := config.Platform{
						Type:    config.PlatformGitLab,
						Account: account,
						Keys:    []config.KeyConfig{},
					}
					persona.Platforms = append(persona.Platforms, platform)
					fmt.Printf("    ✓ Added GitLab.com account: %s\n", account)
				}
			}
		}

		// Prompt for self-hosted GitLab accounts (if any discovered from repos)
		for _, baseURL := range gitlabPrivateBaseURLs {
			fmt.Printf("\n  Add GitLab account for %s? (y/n): ", baseURL)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response == "y" || response == "yes" {
				fmt.Printf("  Enter your username for %s: ", baseURL)
				account, _ := reader.ReadString('\n')
				account = strings.TrimSpace(account)

				if account != "" {
					platform := config.Platform{
						Type:    config.PlatformGitLab,
						Account: account,
						BaseURL: baseURL,
						Keys:    []config.KeyConfig{},
					}
					persona.Platforms = append(persona.Platforms, platform)
					fmt.Printf("    ✓ Added GitLab account: %s (%s)\n", account, baseURL)
				}
			}
		}

		// Option to manually add platform if none discovered or user wants to add more
		for {
			fmt.Print("\nAdd another platform manually? (y/n): ")
			response, _ = reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response != "y" && response != "yes" {
				break
			}

			// Manual platform addition
			manualPlatform, err := promptForPlatform(reader)
			if err != nil {
				fmt.Printf("  Error: %v\n", err)
				continue
			}
			persona.Platforms = append(persona.Platforms, *manualPlatform)
			fmt.Printf("    ✓ Added %s/%s\n", manualPlatform.Type, manualPlatform.Account)
		}

		if len(persona.Platforms) > 0 {
			cfg.Personas = append(cfg.Personas, persona)
			fmt.Printf("\n  ✅ Created persona '%s' with %d platform(s)\n\n", persona.Name, len(persona.Platforms))
		} else {
			fmt.Printf("\n  ⚠️  No platforms added, skipping persona '%s'\n\n", persona.Name)
		}
	}

	if len(cfg.Personas) == 0 {
		fmt.Println("No personas created. Configuration file will be empty.")
		fmt.Println("Run 'git-keys init' to start fresh.")
		return nil
	}

	// Save configuration
	if err := mgr.Save(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("\n✅ Configuration created with %d persona(s)\n", len(cfg.Personas))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Run 'git-keys plan' to preview changes")
	fmt.Println("  2. Run 'git-keys apply' to generate new keys and update SSH config")

	return nil
}
