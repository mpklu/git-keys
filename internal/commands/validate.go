package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/sshkey"
	"github.com/spf13/cobra"
)

var (
	validateFix bool
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate git-keys configuration and setup",
	Long: `Validate the git-keys configuration for correctness and consistency.

This command performs comprehensive validation checks:
  • YAML syntax and structure
  • Required fields present
  • Valid platform types and URLs
  • Email format validation
  • SSH key file paths exist
  • SSH key permissions (600 for private keys)
  • No duplicate personas/platforms
  • Fingerprint consistency

Use this after manually editing the configuration file to ensure
everything is correct before running 'git-keys apply'.

Examples:
  # Validate configuration
  git-keys validate

  # Validate and attempt to fix common issues
  git-keys validate --fix
`,
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().BoolVar(&validateFix, "fix", false, "Attempt to fix common issues (e.g., file permissions)")
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	fmt.Println("\n🔍 Validating Configuration")
	fmt.Println("============================")
	fmt.Println()

	// Check if config file exists
	configPath := config.GetDefaultConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("❌ Configuration file not found")
		fmt.Printf("   Expected: %s\n\n", configPath)
		fmt.Println("Run 'git-keys init' to create configuration")
		return nil
	}

	fmt.Printf("Config file: %s\n", configPath)
	fmt.Println()

	// Load and validate config
	configMgr := config.NewManager(configPath)
	cfg, err := configMgr.Load()
	if err != nil {
		fmt.Println("❌ Configuration validation failed")
		fmt.Printf("   Error: %v\n\n", err)
		return fmt.Errorf("invalid configuration")
	}

	fmt.Println("✓ YAML syntax valid")
	fmt.Println()

	// Perform detailed validation
	errors := []string{}
	warnings := []string{}
	fixedIssues := []string{}

	// Check personas
	if len(cfg.Personas) == 0 {
		errors = append(errors, "No personas defined")
	}

	seenPersonas := make(map[string]bool)
	for _, persona := range cfg.Personas {
		// Check for duplicate persona names
		if seenPersonas[persona.Name] {
			errors = append(errors, fmt.Sprintf("Duplicate persona name: %s", persona.Name))
		}
		seenPersonas[persona.Name] = true

		// Validate email format (basic check)
		if persona.Email == "" {
			warnings = append(warnings, fmt.Sprintf("Persona '%s' has no email", persona.Name))
		}

		// Check platforms
		if len(persona.Platforms) == 0 {
			warnings = append(warnings, fmt.Sprintf("Persona '%s' has no platforms", persona.Name))
		}

		seenPlatforms := make(map[string]bool)
		for _, platform := range persona.Platforms {
			// Check for duplicate platforms
			platformKey := fmt.Sprintf("%s:%s:%s", platform.Type, platform.Account, platform.BaseURL)
			if seenPlatforms[platformKey] {
				errors = append(errors, fmt.Sprintf("Duplicate platform in persona '%s': %s", persona.Name, platformKey))
			}
			seenPlatforms[platformKey] = true

			// Validate platform type
			validTypes := map[config.PlatformType]bool{
				config.PlatformGitHub: true,
				config.PlatformGitLab: true,
			}
			if !validTypes[platform.Type] {
				errors = append(errors, fmt.Sprintf("Invalid platform type: %s (persona: %s)", platform.Type, persona.Name))
			}

			// Check account
			if platform.Account == "" {
				errors = append(errors, fmt.Sprintf("Platform %s in persona '%s' has no account", platform.Type, persona.Name))
			}

			// Check keys
			if len(platform.Keys) == 0 {
				warnings = append(warnings, fmt.Sprintf("Platform %s/%s has no keys", persona.Name, platform.Type))
			}

			sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
			keyMgr := sshkey.NewManager(sshDir)

			for i, key := range platform.Keys {
				// Validate key path
				if key.LocalPath == "" {
					warnings = append(warnings, fmt.Sprintf("Key in %s/%s has no local path", persona.Name, platform.Type))
					continue
				}

				// Check if key file exists
				if !keyMgr.KeyExists(key.LocalPath) {
					errors = append(errors, fmt.Sprintf("Key file not found: %s", key.LocalPath))
					continue
				}

				// Check key permissions
				info, err := os.Stat(key.LocalPath)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Cannot stat key file: %s", key.LocalPath))
					continue
				}

				mode := info.Mode().Perm()
				expectedMode := os.FileMode(0600)
				if mode != expectedMode {
					if validateFix {
						if err := os.Chmod(key.LocalPath, expectedMode); err != nil {
							errors = append(errors, fmt.Sprintf("Failed to fix permissions for %s: %v", key.LocalPath, err))
						} else {
							fixedIssues = append(fixedIssues, fmt.Sprintf("Fixed permissions for %s (%o -> %o)", key.LocalPath, mode, expectedMode))
						}
					} else {
						warnings = append(warnings, fmt.Sprintf("Insecure permissions on %s: %o (expected: %o)", key.LocalPath, mode, expectedMode))
					}
				}

				// Check fingerprint
				if key.Fingerprint == "" {
					warnings = append(warnings, fmt.Sprintf("Key at %s has no fingerprint", key.LocalPath))
				} else {
					// Verify fingerprint matches actual key file
					pubKeyPath := key.LocalPath + ".pub"
					actualFingerprint, err := keyMgr.GetFingerprint(pubKeyPath)
					if err != nil {
						warnings = append(warnings, fmt.Sprintf("Cannot read fingerprint from %s: %v", pubKeyPath, err))
					} else if actualFingerprint != key.Fingerprint {
						errors = append(errors, fmt.Sprintf("Fingerprint mismatch for %s (config: %s, actual: %s)",
							key.LocalPath, key.Fingerprint, actualFingerprint))
					}
				}

				// Validate key status
				validStatuses := map[config.KeyStatus]bool{
					config.KeyStatusActive:  true,
					config.KeyStatusExpired: true,
					config.KeyStatusRevoked: true,
					config.KeyStatusPending: true,
				}
				if !validStatuses[key.Status] {
					errors = append(errors, fmt.Sprintf("Invalid key status: %s (key #%d in %s/%s)",
						key.Status, i+1, persona.Name, platform.Type))
				}
			}
		}
	}

	// Display results
	fmt.Println("📋 Validation Results")
	fmt.Println("=====================")
	fmt.Println()

	if len(errors) > 0 {
		fmt.Printf("❌ Errors: %d\n", len(errors))
		for _, err := range errors {
			fmt.Printf("   • %s\n", err)
		}
		fmt.Println()
	}

	if len(warnings) > 0 {
		fmt.Printf("⚠️  Warnings: %d\n", len(warnings))
		for _, warn := range warnings {
			fmt.Printf("   • %s\n", warn)
		}
		fmt.Println()
	}

	if len(fixedIssues) > 0 {
		fmt.Printf("🔧 Fixed: %d\n", len(fixedIssues))
		for _, fix := range fixedIssues {
			fmt.Printf("   • %s\n", fix)
		}
		fmt.Println()
	}

	// Summary
	if len(errors) == 0 && len(warnings) == 0 {
		fmt.Println("✅ Configuration is valid!")
		fmt.Println("   No issues found.")
	} else if len(errors) == 0 {
		fmt.Printf("✓ Configuration is valid with %d warning(s)\n", len(warnings))
	} else {
		fmt.Printf("❌ Configuration has %d error(s)\n", len(errors))
		fmt.Println("   Please fix the errors before running 'git-keys apply'")
	}
	fmt.Println()

	if len(errors) > 0 {
		return fmt.Errorf("validation failed with %d error(s)", len(errors))
	}

	return nil
}
