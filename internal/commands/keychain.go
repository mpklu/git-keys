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
	keychainAll bool
)

var keychainCmd = &cobra.Command{
	Use:   "keychain",
	Short: "Manage SSH keys in macOS Keychain",
	Long: `Add or remove git-keys managed SSH keys from the macOS Keychain and SSH agent.

This command helps you manage which SSH keys are loaded in the SSH agent and
stored in the macOS Keychain for automatic authentication.

Subcommands:
  add     - Add keys to Keychain and SSH agent
  remove  - Remove keys from SSH agent

Examples:
  # Interactively add keys
  git-keys keychain add

  # Add all keys without prompts
  git-keys keychain add --all

  # Remove all keys from agent
  git-keys keychain remove --all

  # Interactively remove keys
  git-keys keychain remove
`,
}

var keychainAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add SSH keys to Keychain and agent",
	Long: `Add git-keys managed SSH keys to the macOS Keychain and SSH agent.

With --all flag, all keys are added automatically.
Without --all, you'll be prompted to confirm each key (default: yes).

Keys added to the Keychain will be automatically loaded after system restart.

Examples:
  # Add all keys
  git-keys keychain add --all

  # Interactively add keys
  git-keys keychain add
`,
	RunE: runKeychainAdd,
}

var keychainRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove SSH keys from agent",
	Long: `Remove git-keys managed SSH keys from the SSH agent.

Note: This only removes keys from the running SSH agent, not from the Keychain.
Keys will be automatically re-loaded from Keychain on next SSH connection.

With --all flag, all keys are removed automatically.
Without --all, you'll be prompted to confirm each key (default: yes).

Examples:
  # Remove all keys from agent
  git-keys keychain remove --all

  # Interactively remove keys
  git-keys keychain remove
`,
	RunE: runKeychainRemove,
}

func init() {
	keychainAddCmd.Flags().BoolVarP(&keychainAll, "all", "a", false, "Add all keys without prompting")
	keychainRemoveCmd.Flags().BoolVarP(&keychainAll, "all", "a", false, "Remove all keys without prompting")

	keychainCmd.AddCommand(keychainAddCmd)
	keychainCmd.AddCommand(keychainRemoveCmd)
	rootCmd.AddCommand(keychainCmd)
}

func runKeychainAdd(cmd *cobra.Command, args []string) error {
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

	// Collect all SSH key paths
	keyPaths := collectKeyPaths(cfg)

	if len(keyPaths) == 0 {
		fmt.Println("No SSH keys found in configuration.")
		fmt.Println("Run 'git-keys apply' to generate keys.")
		return nil
	}

	fmt.Printf("\n🔑 Adding SSH Keys to Keychain\n")
	fmt.Printf("==============================\n\n")

	reader := bufio.NewReader(os.Stdin)
	addedCount := 0
	skippedCount := 0

	for _, keyPath := range keyPaths {
		keyName := filepath.Base(keyPath)

		// Check if key file exists
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			logger.Warn("Key file not found: %s", keyPath)
			skippedCount++
			continue
		}

		// Check if key is already in agent
		inAgent := isKeyInAgent(keyPath)
		status := ""
		if inAgent {
			status = " (already in agent)"
		}

		if !keychainAll {
			// Interactive mode - prompt for confirmation
			fmt.Printf("Add %s to Keychain?%s [Y/n]: ", keyName, status)
			response, _ := reader.ReadString('\n')
			response = strings.ToLower(strings.TrimSpace(response))

			if response == "n" || response == "no" {
				fmt.Printf("  ⊘ Skipped\n\n")
				skippedCount++
				continue
			}
		}

		// Add key to Keychain
		if err := addKeyToKeychain(keyPath); err != nil {
			logger.Warn("Failed to add %s: %v", keyName, err)
			skippedCount++
			continue
		}

		fmt.Printf("  ✓ Added %s\n", keyName)
		if !keychainAll {
			fmt.Println()
		}
		addedCount++
	}

	// Summary
	fmt.Printf("\n✅ Summary: %d added, %d skipped\n\n", addedCount, skippedCount)

	if addedCount > 0 {
		fmt.Println("Keys have been added to the SSH agent and macOS Keychain.")
		fmt.Println("They will be automatically loaded after system restart.")
		fmt.Println("\nVerify with: ssh-add -l")

		// Prompt to test SSH connections
		fmt.Print("\nTest SSH connections to verify setup? [Y/n]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "" || response == "y" || response == "yes" {
			fmt.Println()
			testSSHConnections(cfg)
		}
	}

	return nil
}

func runKeychainRemove(cmd *cobra.Command, args []string) error {
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

	// Collect all SSH key paths
	keyPaths := collectKeyPaths(cfg)

	if len(keyPaths) == 0 {
		fmt.Println("No SSH keys found in configuration.")
		return nil
	}

	fmt.Printf("\n🔑 Removing SSH Keys from Agent\n")
	fmt.Printf("================================\n\n")

	reader := bufio.NewReader(os.Stdin)
	removedCount := 0
	skippedCount := 0

	for _, keyPath := range keyPaths {
		keyName := filepath.Base(keyPath)

		// Check if key is in agent
		inAgent := isKeyInAgent(keyPath)
		if !inAgent {
			if !keychainAll {
				fmt.Printf("%s (not in agent)\n", keyName)
				fmt.Printf("  ⊘ Skipped\n\n")
			}
			skippedCount++
			continue
		}

		if !keychainAll {
			// Interactive mode - prompt for confirmation
			fmt.Printf("Remove %s from agent? [Y/n]: ", keyName)
			response, _ := reader.ReadString('\n')
			response = strings.ToLower(strings.TrimSpace(response))

			if response == "n" || response == "no" {
				fmt.Printf("  ⊘ Skipped\n\n")
				skippedCount++
				continue
			}
		}

		// Remove key from agent
		if err := removeKeyFromAgent(keyPath); err != nil {
			logger.Warn("Failed to remove %s: %v", keyName, err)
			skippedCount++
			continue
		}

		fmt.Printf("  ✓ Removed %s\n", keyName)
		if !keychainAll {
			fmt.Println()
		}
		removedCount++
	}

	// Summary
	fmt.Printf("\n✅ Summary: %d removed, %d skipped\n\n", removedCount, skippedCount)

	if removedCount > 0 {
		fmt.Println("Keys have been removed from the SSH agent.")
		fmt.Println("Note: They remain in Keychain and will reload on next SSH connection.")
		fmt.Println("\nVerify with: ssh-add -l")
	}

	return nil
}

// collectKeyPaths gathers all SSH key paths from the configuration
func collectKeyPaths(cfg *config.Config) []string {
	var keyPaths []string
	homeDir, _ := os.UserHomeDir()

	for _, persona := range cfg.Personas {
		for _, platform := range persona.Platforms {
			for _, key := range platform.Keys {
				// Expand path
				keyPath := key.LocalPath
				if strings.HasPrefix(keyPath, "~/") {
					keyPath = filepath.Join(homeDir, keyPath[2:])
				} else if !filepath.IsAbs(keyPath) {
					keyPath = filepath.Join(homeDir, ".ssh", keyPath)
				}

				keyPaths = append(keyPaths, keyPath)
			}
		}
	}

	return keyPaths
}

// addKeyToKeychain adds an SSH key to the macOS Keychain and SSH agent
func addKeyToKeychain(keyPath string) error {
	cmd := exec.Command("ssh-add", "--apple-use-keychain", keyPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// removeKeyFromAgent removes an SSH key from the SSH agent
func removeKeyFromAgent(keyPath string) error {
	cmd := exec.Command("ssh-add", "-d", keyPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// isKeyInAgent checks if a key is currently loaded in the SSH agent
func isKeyInAgent(keyPath string) bool {
	// Get list of loaded keys
	cmd := exec.Command("ssh-add", "-l")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Get the public key fingerprint
	pubKeyPath := keyPath + ".pub"
	fingerprintCmd := exec.Command("ssh-keygen", "-lf", pubKeyPath)
	fingerprintOutput, err := fingerprintCmd.Output()
	if err != nil {
		return false
	}

	fingerprint := strings.Fields(string(fingerprintOutput))
	if len(fingerprint) < 2 {
		return false
	}

	// Check if fingerprint exists in agent list
	return strings.Contains(string(output), fingerprint[1])
}

// testSSHConnections tests SSH connections to all configured platforms
func testSSHConnections(cfg *config.Config) {
	fmt.Println("Testing SSH connections...")
	fmt.Println()

	successCount := 0
	failureCount := 0

	for _, persona := range cfg.Personas {
		for _, platform := range persona.Platforms {
			// Build SSH host based on platform
			var hostname string
			sanitizedPersona := sanitizeHostname(persona.Name)

			switch platform.Type {
			case "github":
				hostname = fmt.Sprintf("github.com.%s", sanitizedPersona)
			case "gitlab":
				// Extract hostname from base_url (e.g., gitlab.company.net from https://gitlab.company.net)
				if platform.BaseURL != "" {
					baseURL := strings.TrimPrefix(platform.BaseURL, "https://")
					baseURL = strings.TrimPrefix(baseURL, "http://")
					baseURL = strings.TrimSuffix(baseURL, "/")
					hostname = fmt.Sprintf("%s.%s", baseURL, sanitizedPersona)
				} else {
					hostname = fmt.Sprintf("gitlab.com.%s", sanitizedPersona)
				}
			default:
				continue // Skip unknown platforms
			}

			// Test SSH connection
			testCmd := exec.Command("ssh", "-T", fmt.Sprintf("git@%s", hostname))
			output, _ := testCmd.CombinedOutput()
			outputStr := strings.TrimSpace(string(output))

			// Check for successful authentication
			// GitHub: "Hi {username}! You've successfully authenticated"
			// GitLab: "Welcome to GitLab, @{username}!"
			if strings.Contains(outputStr, "successfully authenticated") || strings.Contains(outputStr, "Welcome to GitLab") {
				fmt.Printf("  ✓ %s (%s): %s\n", platform.Account, platform.Type, extractAuthMessage(outputStr))
				successCount++
			} else {
				fmt.Printf("  ✗ %s (%s): Authentication failed\n", platform.Account, platform.Type)
				if outputStr != "" {
					fmt.Printf("    %s\n", outputStr)
				}
				failureCount++
			}
		}
	}

	fmt.Println()
	if failureCount == 0 {
		fmt.Printf("✅ All %d connection(s) successful!\n\n", successCount)
	} else {
		fmt.Printf("⚠️  %d successful, %d failed\n\n", successCount, failureCount)
	}
}

// extractAuthMessage extracts the relevant authentication message from SSH output
func extractAuthMessage(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "successfully authenticated") {
			// Extract "Hi {username}!" part
			if strings.HasPrefix(line, "Hi ") {
				parts := strings.Split(line, "!")
				if len(parts) > 0 {
					return parts[0] + "!"
				}
			}
			return line
		}
		if strings.Contains(line, "Welcome to GitLab") {
			return line
		}
	}
	return strings.Split(output, "\n")[0]
}
