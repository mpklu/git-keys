package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
	"github.com/kunlu/git-keys/internal/platform"
	"github.com/kunlu/git-keys/internal/sshconfig"
	"github.com/spf13/cobra"
)

var (
	importInteractive bool
	importDryRun      bool
	importAuto        bool
)

// KeyImport represents a key to be imported
type KeyImport struct {
	SourcePath  string
	Platform    string
	PersonaName string
	Email       string
	BaseURL     string
	Action      string // "move", "copy", or "reference"
	TargetPath  string
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import existing SSH keys into git-keys management",
	Long: `Interactive wizard to import existing SSH keys and configs into git-keys management.

This provides a smooth migration path for users with existing SSH setups.

The wizard will:
  1. Discover existing SSH keys
  2. Help you map keys to personas and platforms
  3. Optionally reorganize keys into git-keys directory structure
  4. Update SSH config with managed blocks
  5. Create or update git-keys configuration

All changes are backed up and reversible.`,
	RunE: runImport,
}

func init() {
	importCmd.Flags().BoolVar(&importInteractive, "interactive", true, "Interactive wizard mode")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Show what would be imported without making changes")
	importCmd.Flags().BoolVar(&importAuto, "auto", false, "Attempt automatic mapping based on SSH config")
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	if importAuto {
		return fmt.Errorf("--auto mode not yet implemented")
	}

	logger.Info("Starting import wizard...")
	fmt.Println()

	// Step 1: Discover existing keys
	fmt.Println("🔍 Discovering existing SSH setup...")
	fmt.Println()

	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	keys, err := scanSSHKeys(sshDir)
	if err != nil {
		return fmt.Errorf("failed to scan SSH keys: %w", err)
	}

	if len(keys) == 0 {
		fmt.Println("No SSH keys found. Nothing to import.")
		fmt.Println()
		fmt.Println("To create a new setup, run: git-keys init")
		return nil
	}

	fmt.Printf("Found %d SSH key(s):\n", len(keys))
	for i, key := range keys {
		usedBy := ""
		if len(key.UsedBy) > 0 {
			usedBy = fmt.Sprintf(" (used by %s)", strings.Join(key.UsedBy, ", "))
		}
		fmt.Printf("  %d. %s%s\n", i+1, key.Path, usedBy)
	}
	fmt.Println()

	// Step 2: Map keys to personas
	fmt.Println("Let's map your existing keys to personas:")
	fmt.Println()

	var imports []KeyImport
	reader := bufio.NewReader(os.Stdin)

	for _, key := range keys {
		fmt.Printf("Key: %s\n", filepath.Base(key.Path))
		if len(key.UsedBy) > 0 {
			fmt.Printf("  Currently used for: %s\n", strings.Join(key.UsedBy, ", "))
		}
		fmt.Println()

		// Ask if user wants to import this key
		importKey := promptYesNo(reader, "  Import this key?")
		if !importKey {
			fmt.Println("  ⊘ Skipping")
			fmt.Println()
			continue
		}

		// Determine platform
		platform := promptPlatform(reader, key)
		if platform == "skip" {
			fmt.Println("  ⊘ Skipping")
			fmt.Println()
			continue
		}

		// Determine persona
		persona := promptString(reader, "  Persona name (e.g., personal, work)")
		if persona == "" {
			persona = "default"
		}

		// Get email
		email := promptString(reader, "  Email for commits")

		// Get base URL for GitLab
		baseURL := ""
		if platform == "gitlab" {
			selfHosted := promptYesNo(reader, "  Is this self-hosted GitLab?")
			if selfHosted {
				baseURL = promptString(reader, "  GitLab URL (e.g., https://gitlab.company.com)")
			}
		}

		imp := KeyImport{
			SourcePath:  key.Path,
			Platform:    platform,
			PersonaName: persona,
			Email:       email,
			BaseURL:     baseURL,
			Action:      "move", // Default action
		}

		imports = append(imports, imp)

		platformDesc := platform
		if baseURL != "" {
			platformDesc = fmt.Sprintf("%s (%s)", platform, baseURL)
		}

		fmt.Printf("  ✓ Will import as: %s/%s\n", platformDesc, persona)
		fmt.Println()
	}

	if len(imports) == 0 {
		fmt.Println("No keys selected for import.")
		return nil
	}

	// Step 3: Key relocation options
	fmt.Println("🔧 Key Management Options:")
	fmt.Println()
	fmt.Println("  git-keys can organize keys in ~/.ssh/git-keys/")
	fmt.Println()
	fmt.Println("  Options:")
	fmt.Println("  1. Move keys to git-keys directory (recommended)")
	fmt.Println("     - Reorganizes keys by persona")
	fmt.Println("     - Updates SSH config automatically")
	fmt.Println()
	fmt.Println("  2. Leave keys in current location")
	fmt.Println("     - git-keys uses existing paths")
	fmt.Println("     - You retain current key locations")
	fmt.Println()
	fmt.Println("  3. Copy keys to git-keys directory")
	fmt.Println("     - Keeps originals as backup")
	fmt.Println("     - git-keys manages copies")
	fmt.Println()

	choice := promptChoice(reader, "  Choice", []string{"1", "2", "3"}, "1")

	action := "move"
	switch choice {
	case "1":
		action = "move"
	case "2":
		action = "reference"
	case "3":
		action = "copy"
	}

	for i := range imports {
		imports[i].Action = action
	}

	// Generate target paths
	gitKeysDir := filepath.Join(sshDir, "git-keys")
	for i := range imports {
		imp := &imports[i]
		if action == "reference" {
			imp.TargetPath = imp.SourcePath
		} else {
			// Generate standardized name
			keyType := "ed25519" // Default, should detect from actual key
			imp.TargetPath = filepath.Join(gitKeysDir, fmt.Sprintf("%s-%s-%s", imp.Platform, imp.PersonaName, keyType))
		}
	}

	// Step 4: Show summary and confirm
	fmt.Println()
	fmt.Println("✅ Import Summary:")
	fmt.Println()
	fmt.Printf("  Keys to import: %d\n", len(imports))
	for _, imp := range imports {
		platformDesc := imp.Platform
		if imp.BaseURL != "" {
			platformDesc = fmt.Sprintf("%s (%s)", imp.Platform, imp.BaseURL)
		}
		fmt.Printf("    ✓ %s/%s (%s)\n", platformDesc, imp.PersonaName, filepath.Base(imp.SourcePath))
		if action != "reference" {
			fmt.Printf("      %s → %s\n", imp.SourcePath, imp.TargetPath)
		}
	}
	fmt.Println()

	if action == "move" {
		fmt.Println("  Actions:")
		fmt.Println("    • Create ~/.ssh/git-keys/ directory")
		fmt.Printf("    • Move %d key(s) to git-keys directory\n", len(imports))
		fmt.Println("    • Update ~/.ssh/config (backup created)")
		fmt.Println("    • Create/update git-keys configuration")
		fmt.Println()
	}

	fmt.Println("  No keys will be deleted.")
	fmt.Println("  All changes are reversible.")
	fmt.Println()

	if importDryRun {
		fmt.Println("  [DRY RUN - no changes made]")
		return nil
	}

	proceed := promptYesNo(reader, "Proceed with import?")
	if !proceed {
		fmt.Println()
		fmt.Println("Import cancelled.")
		return nil
	}

	// Step 5: Execute import
	fmt.Println()
	fmt.Println("⚙️  Executing import...")
	fmt.Println()

	if err := executeImport(imports, sshDir, gitKeysDir); err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ Import complete!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Run: git-keys plan")
	fmt.Println("     View your configuration")
	fmt.Println()
	fmt.Println("  2. Set platform tokens:")
	fmt.Println("     export GITHUB_API_TOKEN='...'")
	fmt.Println("     export GITLAB_TOKEN='...'")
	fmt.Println()
	fmt.Println("  3. Apply configuration:")
	fmt.Println("     git-keys apply")
	fmt.Println()

	return nil
}

func executeImport(imports []KeyImport, sshDir, gitKeysDir string) error {
	// Create git-keys directory if needed
	if err := os.MkdirAll(gitKeysDir, 0700); err != nil {
		return fmt.Errorf("creating git-keys directory: %w", err)
	}

	// Get or create machine info
	plat, err := platform.NewPlatform()
	if err != nil {
		return fmt.Errorf("getting platform info: %w", err)
	}

	machineID, err := plat.GetMachineID()
	if err != nil {
		return fmt.Errorf("getting machine ID: %w", err)
	}

	machineName, err := plat.GetMachineName()
	if err != nil {
		machineName = "unknown"
	}

	osVersion, _ := plat.GetOSVersion()

	// Load or create config
	configPath := config.GetDefaultConfigPath()
	mgr := config.NewManager(configPath)

	var cfg *config.Config
	if mgr.Exists() {
		cfg, err = mgr.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	} else {
		// Create new config
		cfg = mgr.CreateDefault(config.Machine{
			ID:        machineID,
			Name:      machineName,
			OS:        plat.GetOS(),
			OSVersion: osVersion,
		})
	}

	fmt.Println("  ✓ Created machine profile")

	// Process each import

	for _, imp := range imports {
		fmt.Printf("  Processing %s/%s...\n", imp.Platform, imp.PersonaName)

		// Find or create persona
		persona := findOrCreatePersona(cfg, imp.PersonaName, imp.Email)

		// Determine platform type
		var platformType config.PlatformType
		if imp.Platform == "github" {
			platformType = config.PlatformGitHub
		} else {
			platformType = config.PlatformGitLab
		}

		// Create platform config
		platformCfg := config.Platform{
			Type:    platformType,
			Account: imp.PersonaName, // Using persona name as account
			BaseURL: imp.BaseURL,
			Keys:    []config.KeyConfig{},
		}

		persona.Platforms = append(persona.Platforms, platformCfg)

		// Handle key relocation
		if imp.Action == "move" || imp.Action == "copy" {
			// Copy/move the private key
			if err := copyOrMoveKey(imp.SourcePath, imp.TargetPath, imp.Action == "move"); err != nil {
				return fmt.Errorf("relocating key: %w", err)
			}

			// Copy/move the public key
			if err := copyOrMoveKey(imp.SourcePath+".pub", imp.TargetPath+".pub", imp.Action == "move"); err != nil {
				return fmt.Errorf("relocating public key: %w", err)
			}

			fmt.Printf("    ✓ %s key to %s\n", strings.Title(imp.Action), imp.TargetPath)
		}
	}

	// Save updated config
	if err := mgr.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("  ✓ Updated configuration")

	// Update SSH config
	if err := updateSSHConfigForImport(imports, sshDir); err != nil {
		logger.Warn("Failed to update SSH config: %v", err)
		fmt.Println("  ⚠ Could not update SSH config automatically")
		fmt.Println("    You may need to update it manually")
	} else {
		fmt.Println("  ✓ Updated SSH config")
	}

	return nil
}

func findOrCreatePersona(cfg *config.Config, name, email string) *config.Persona {
	for i := range cfg.Personas {
		if cfg.Personas[i].Name == name {
			return &cfg.Personas[i]
		}
	}

	// Create new persona
	persona := config.Persona{
		Name:      name,
		Email:     email,
		Platforms: []config.Platform{},
	}
	cfg.Personas = append(cfg.Personas, persona)

	// Return pointer to the newly added persona
	return &cfg.Personas[len(cfg.Personas)-1]
}

func copyOrMoveKey(src, dst string, move bool) error {
	// Read source
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading source: %w", err)
	}

	// Write destination
	// Determine permissions from source
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	if err := os.WriteFile(dst, data, srcInfo.Mode()); err != nil {
		return fmt.Errorf("writing destination: %w", err)
	}

	// If moving, remove source
	if move {
		// Create archive directory for backup
		archiveDir := filepath.Join(filepath.Dir(src), "archive")
		os.MkdirAll(archiveDir, 0700)

		// Move to archive with timestamp
		archivePath := filepath.Join(archiveDir, filepath.Base(src)+"."+time.Now().Format("2006-01-02"))
		os.Rename(src, archivePath)
	}

	return nil
}

func updateSSHConfigForImport(imports []KeyImport, sshDir string) error {
	configPath := filepath.Join(sshDir, "config")
	mgr := sshconfig.NewManager(configPath)

	// Build SSH config entries for all imports
	var entries []sshconfig.Entry

	for _, imp := range imports {
		// Determine the host pattern based on platform
		host := imp.Platform + ".com"
		if imp.Platform == "github" {
			host = "github.com"
		} else if imp.Platform == "gitlab" {
			if imp.BaseURL != "" {
				// Extract hostname from URL
				host = strings.TrimPrefix(imp.BaseURL, "https://")
				host = strings.TrimPrefix(host, "http://")
				host = strings.TrimSuffix(host, "/")
			} else {
				host = "gitlab.com"
			}
		}

		entry := sshconfig.Entry{
			Host:         host,
			HostName:     host,
			IdentityFile: imp.TargetPath,
			User:         "git",
		}

		entries = append(entries, entry)
	}

	// Add all entries in one managed block
	if err := mgr.AddOrUpdateEntry("git-keys-imported", entries); err != nil {
		return err
	}

	return nil
}

// Helper functions for prompts

func promptYesNo(reader *bufio.Reader, prompt string) bool {
	for {
		fmt.Printf("%s (y/n): ", prompt)
		response, _ := reader.ReadString('\n')
		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" || response == "yes" {
			return true
		} else if response == "n" || response == "no" {
			return false
		}

		fmt.Println("Please enter 'y' or 'n'")
	}
}

func promptString(reader *bufio.Reader, prompt string) string {
	fmt.Printf("%s: ", prompt)
	response, _ := reader.ReadString('\n')
	return strings.TrimSpace(response)
}

func promptChoice(reader *bufio.Reader, prompt string, choices []string, defaultChoice string) string {
	for {
		choiceStr := strings.Join(choices, "/")
		fmt.Printf("%s [%s]: ", prompt, choiceStr)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(response)

		if response == "" {
			return defaultChoice
		}

		for _, choice := range choices {
			if response == choice {
				return response
			}
		}

		fmt.Printf("Please enter one of: %s\n", choiceStr)
	}
}

func promptPlatform(reader *bufio.Reader, key DiscoveredKey) string {
	// Try to guess from key usage
	defaultPlatform := "github"
	for _, host := range key.UsedBy {
		if strings.Contains(host, "gitlab") {
			defaultPlatform = "gitlab"
			break
		}
	}

	fmt.Printf("  Platform [github/gitlab/other/skip] (default: %s): ", defaultPlatform)
	response, _ := reader.ReadString('\n')
	response = strings.ToLower(strings.TrimSpace(response))

	if response == "" {
		return defaultPlatform
	}

	validPlatforms := map[string]bool{
		"github": true,
		"gitlab": true,
		"other":  true,
		"skip":   true,
	}

	if validPlatforms[response] {
		return response
	}

	return defaultPlatform
}
