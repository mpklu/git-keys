package commands

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
	"github.com/spf13/cobra"
)

var (
	setupGitDryRun bool
)

var setupGitCmd = &cobra.Command{
	Use:   "setup-git",
	Short: "Configure or reconfigure git identity settings for personas",
	Long: `Create or update git configuration files for each persona with automatic identity switching.

Note: This is typically done automatically by 'git-keys apply'. Use this command to:
  - Reconfigure directory patterns for personas
  - Update git configuration without regenerating keys
  - Fix or modify existing git config setup

This command will:
  1. Create persona-specific git config files (e.g., ~/.gitconfig-personal)
  2. Add conditional includeIf entries to ~/.gitconfig
  3. Configure git user name/email for each persona
  4. Set up SSH URL rewrites to use persona-specific hosts

After running this, your git commits will automatically use the correct identity
and SSH key based on which directory you're working in.

You'll be prompted to specify a directory pattern for each persona.

Examples:
  # Reconfigure git setup for all personas
  git-keys setup-git

  # Preview what would be created
  git-keys setup-git --dry-run
`,
	RunE: runSetupGit,
}

func init() {
	setupGitCmd.Flags().BoolVar(&setupGitDryRun, "dry-run", false, "Show what would be created without making changes")
	rootCmd.AddCommand(setupGitCmd)
}

func runSetupGit(cmd *cobra.Command, args []string) error {
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

	if len(cfg.Personas) == 0 {
		return fmt.Errorf("no personas configured. Run 'git-keys init' first")
	}

	fmt.Println("\n⚙️  Git Configuration Setup")
	fmt.Println("=========================")
	fmt.Println()

	if setupGitDryRun {
		fmt.Println("🔍 DRY RUN MODE - No changes will be made\n")
	}

	// Collect directory patterns for each platform
	type platformEntry struct {
		personaIdx  int
		platformIdx int
		persona     *config.Persona
		platform    *config.Platform
		configName  string
	}

	var platforms []platformEntry
	reader := bufio.NewReader(os.Stdin)
	configChanged := false

	// Build list of all platforms across all personas
	for personaIdx := range cfg.Personas {
		persona := &cfg.Personas[personaIdx]
		for platformIdx := range persona.Platforms {
			platform := &persona.Platforms[platformIdx]
			platformID := fmt.Sprintf("%s-%s", string(platform.Type), platform.Account)
			configName := fmt.Sprintf(".gitconfig-%s-%s", persona.Name, platformID)

			platforms = append(platforms, platformEntry{
				personaIdx:  personaIdx,
				platformIdx: platformIdx,
				persona:     persona,
				platform:    platform,
				configName:  configName,
			})
		}
	}

	personaDirs := make(map[string]string)

	for _, entry := range platforms {
		persona := entry.persona
		platform := entry.platform
		platformID := fmt.Sprintf("%s/%s", platform.Type, platform.Account)

		// Show existing pattern if available
		existingPattern := ""
		if platform.GitDir != "" {
			existingPattern = platform.GitDir
		}

		fmt.Printf("📋 %s <%s> - %s\n", persona.Name, persona.Email, platformID)
		if existingPattern != "" {
			fmt.Printf("   Current pattern: %s\n", existingPattern)
		}
		fmt.Printf("   Enter directory pattern (e.g., ~/Projects/%s/", platform.Account)
		if existingPattern != "" {
			fmt.Print(", or press Enter to keep current): ")
		} else {
			fmt.Print("): ")
		}

		pattern, _ := reader.ReadString('\n')
		pattern = strings.TrimSpace(pattern)

		// Use existing if no new input
		if pattern == "" {
			if existingPattern != "" {
				pattern = existingPattern
			} else {
				fmt.Printf("   ⚠️  Skipping (no pattern provided)\n\n")
				continue
			}
		}

		// Expand ~ to home directory
		if strings.HasPrefix(pattern, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				pattern = filepath.Join(home, pattern[2:])
			}
		}

		// Ensure pattern ends with /
		if !strings.HasSuffix(pattern, "/") {
			pattern = pattern + "/"
		}

		// Update config if changed
		if pattern != platform.GitDir {
			platform.GitDir = pattern
			configChanged = true
		}

		key := fmt.Sprintf("%s-%s-%s", persona.Name, platform.Type, platform.Account)
		personaDirs[key] = pattern
		fmt.Printf("   ✓ Will configure for: %s\n\n", pattern)
	}

	if len(personaDirs) == 0 {
		fmt.Println("No personas configured. Exiting.")
		return nil
	}

	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	globalGitConfig := filepath.Join(home, ".gitconfig")

	// Backup global gitconfig
	if !setupGitDryRun {
		backupPath := globalGitConfig + ".backup-git-keys"
		if _, err := os.Stat(globalGitConfig); err == nil {
			content, err := os.ReadFile(globalGitConfig)
			if err == nil {
				os.WriteFile(backupPath, content, 0644)
				fmt.Printf("💾 Backed up ~/.gitconfig to ~/.gitconfig.backup-git-keys\n\n")
			}
		}
	}

	// Create platform-specific config files and includeIf entries
	var includeEntries []string

	for _, entry := range platforms {
		persona := entry.persona
		platform := entry.platform
		key := fmt.Sprintf("%s-%s-%s", persona.Name, platform.Type, platform.Account)

		dirPattern, ok := personaDirs[key]
		if !ok {
			continue
		}

		// Create platform config file
		configPath := filepath.Join(home, entry.configName)

		if setupGitDryRun {
			fmt.Printf("Would create: %s\n", configPath)
		} else {
			if err := createPlatformGitConfig(persona, platform, configPath); err != nil {
				logger.Warn("Failed to create config for %s/%s-%s: %v", persona.Name, platform.Type, platform.Account, err)
				continue
			}
			fmt.Printf("✓ Created: %s\n", configPath)
		}

		// Create includeIf entry
		includeEntry := fmt.Sprintf("[includeIf \"gitdir:%s\"]\n\tpath = %s\n", dirPattern, configPath)
		includeEntries = append(includeEntries, includeEntry)
	}

	// Update global gitconfig with includeIf entries
	if len(includeEntries) > 0 {
		if setupGitDryRun {
			fmt.Println("\nWould add to ~/.gitconfig:")
			fmt.Println("---")
			for _, entry := range includeEntries {
				fmt.Print(entry)
			}
			fmt.Println("---")
		} else {
			if err := addIncludeIfEntries(globalGitConfig, includeEntries); err != nil {
				return fmt.Errorf("failed to update ~/.gitconfig: %w", err)
			}
			fmt.Printf("\n✓ Updated ~/.gitconfig with %d persona configuration(s)\n", len(includeEntries))
		}
	}

	// Save config if gitdir was updated
	if configChanged && !setupGitDryRun {
		if err := mgr.Save(cfg); err != nil {
			logger.Warn("Failed to save config: %v", err)
		} else {
			fmt.Printf("✓ Saved directory patterns to configuration\n\n")
		}
	}

	if setupGitDryRun {
		fmt.Println("\n✓ Dry run complete. Run without --dry-run to apply changes.")
	} else {
		fmt.Println("\n✅ Git configuration setup complete!")
		fmt.Println("\nYour git commits will now automatically use the correct identity")
		fmt.Println("and SSH key based on your working directory.")
		fmt.Println("\nTest it:")
		for name, dir := range personaDirs {
			fmt.Printf("  cd %s\n", dir)
			fmt.Printf("  git config user.email  # Should show persona '%s'\n", name)
			fmt.Println()
		}
	}

	return nil
}

func createPlatformGitConfig(persona *config.Persona, platform *config.Platform, configPath string) error {
	var content strings.Builder

	// User identity
	content.WriteString(fmt.Sprintf("# Git configuration for %s <%s>\n", persona.Name, persona.Email))
	content.WriteString(fmt.Sprintf("# Platform: %s/%s\n", platform.Type, platform.Account))
	content.WriteString(fmt.Sprintf("# Managed by git-keys\n\n"))
	content.WriteString("[user]\n")
	content.WriteString(fmt.Sprintf("\tname = %s\n", persona.Name))
	content.WriteString(fmt.Sprintf("\temail = %s\n\n", persona.Email))

	// URL rewrites for SSH hosts (platform-specific)
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

func addIncludeIfEntries(gitConfigPath string, entries []string) error {
	// Read existing gitconfig
	var existingContent string
	if data, err := os.ReadFile(gitConfigPath); err == nil {
		existingContent = string(data)
	}

	// Check if git-keys managed section already exists
	managedMarker := "# BEGIN git-keys managed conditional includes"
	endMarker := "# END git-keys managed conditional includes"

	var newContent string

	if strings.Contains(existingContent, managedMarker) {
		// Replace existing managed section
		startIdx := strings.Index(existingContent, managedMarker)
		endIdx := strings.Index(existingContent, endMarker)

		if endIdx > startIdx {
			// Remove old managed section
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

func removeGitKeysConfig() error {
	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	globalGitConfig := filepath.Join(home, ".gitconfig")

	// Read existing gitconfig
	data, err := os.ReadFile(globalGitConfig)
	if err != nil {
		return err
	}

	content := string(data)
	managedMarker := "# BEGIN git-keys managed conditional includes"
	endMarker := "# END git-keys managed conditional includes"

	if !strings.Contains(content, managedMarker) {
		return nil // Nothing to remove
	}

	// Remove managed section
	startIdx := strings.Index(content, managedMarker)
	endIdx := strings.Index(content, endMarker)

	if endIdx > startIdx {
		before := content[:startIdx]
		after := content[endIdx+len(endMarker):]

		newContent := strings.TrimRight(before, "\n") + "\n" + strings.TrimLeft(after, "\n")

		return os.WriteFile(globalGitConfig, []byte(newContent), 0644)
	}

	return nil
}

// Helper function to check if git is installed
func isGitInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}
