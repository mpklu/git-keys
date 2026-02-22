package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/sshkey"
	"github.com/spf13/cobra"
)

var (
	statusVerbose bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the health and status of managed SSH keys",
	Long: `Display comprehensive status information about git-keys managed SSH keys.

This command shows:
  • Configuration file status
  • Persona and platform overview
  • Key status (active, rotated, expired)
  • Key age and rotation recommendations
  • SSH config integration status
  • File existence verification

Use this to quickly verify your git-keys setup is healthy and identify
keys that may need rotation or attention.

Examples:
  # Show overview status
  git-keys status

  # Show detailed status
  git-keys status --verbose
`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVarP(&statusVerbose, "verbose", "v", false, "Show detailed status information")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("\n📊 Git-Keys Status")
	fmt.Println("==================")
	fmt.Println()

	// Check configuration file
	configPath := config.GetDefaultConfigPath()
	configMgr := config.NewManager(configPath)

	cfg, err := configMgr.Load()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("❌ Configuration Status: Not initialized")
			fmt.Printf("   Config file not found: %s\n\n", configPath)
			fmt.Println("Run 'git-keys init' to get started")
			return nil
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("✓ Configuration Status: OK")
	fmt.Printf("  Config file: %s\n\n", configPath)

	// Overview
	totalPersonas := len(cfg.Personas)
	totalPlatforms := 0
	totalKeys := 0
	activeKeys := 0
	revokedKeys := 0
	expiredKeys := 0

	for _, persona := range cfg.Personas {
		totalPlatforms += len(persona.Platforms)
		for _, platform := range persona.Platforms {
			totalKeys += len(platform.Keys)
			for _, key := range platform.Keys {
				switch key.Status {
				case config.KeyStatusActive:
					activeKeys++
				case config.KeyStatusRevoked:
					revokedKeys++
				case config.KeyStatusExpired:
					expiredKeys++
				}
			}
		}
	}

	fmt.Println("📈 Overview")
	fmt.Println("===========")
	fmt.Printf("Personas: %d\n", totalPersonas)
	fmt.Printf("Platforms: %d\n", totalPlatforms)
	fmt.Printf("Total Keys: %d\n", totalKeys)
	fmt.Printf("  Active: %d\n", activeKeys)
	if revokedKeys > 0 {
		fmt.Printf("  Revoked: %d\n", revokedKeys)
	}
	if expiredKeys > 0 {
		fmt.Printf("  Expired: %d ⚠️\n", expiredKeys)
	}
	fmt.Println()

	// Health checks
	fmt.Println("🏥 Health Checks")
	fmt.Println("================")

	warnings := []string{}
	errors := []string{}

	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	keyMgr := sshkey.NewManager(sshDir)

	keysNeedingRotation := 0
	missingKeyFiles := 0

	for _, persona := range cfg.Personas {
		for _, platform := range persona.Platforms {
			for _, key := range platform.Keys {
				// Check key file exists
				if key.LocalPath != "" {
					if !keyMgr.KeyExists(key.LocalPath) {
						missingKeyFiles++
						if statusVerbose {
							errors = append(errors, fmt.Sprintf("Missing key file: %s", key.LocalPath))
						}
					}
				}

				// Check key age for rotation
				if key.Status == config.KeyStatusActive && !key.CreatedAt.IsZero() {
					age := time.Since(key.CreatedAt)
					daysSinceCreation := int(age.Hours() / 24)

					// Recommend rotation after 90 days
					if daysSinceCreation > 90 {
						keysNeedingRotation++
						if statusVerbose {
							warnings = append(warnings, fmt.Sprintf("Key needs rotation: %s/%s (age: %d days)",
								persona.Name, platform.Type, daysSinceCreation))
						}
					}
				}
			}
		}
	}

	// Display health summary
	healthOK := true
	if missingKeyFiles > 0 {
		healthOK = false
		fmt.Printf("❌ Missing key files: %d\n", missingKeyFiles)
	}
	if expiredKeys > 0 {
		healthOK = false
		fmt.Printf("❌ Expired keys: %d\n", expiredKeys)
	}
	if keysNeedingRotation > 0 {
		fmt.Printf("⚠️  Keys needing rotation (>90 days): %d\n", keysNeedingRotation)
	}

	if healthOK && keysNeedingRotation == 0 {
		fmt.Println("✓ All checks passed")
	}
	fmt.Println()

	// Show warnings and errors in verbose mode
	if statusVerbose {
		if len(errors) > 0 {
			fmt.Println("❌ Errors")
			fmt.Println("=========")
			for _, err := range errors {
				fmt.Printf("  • %s\n", err)
			}
			fmt.Println()
		}

		if len(warnings) > 0 {
			fmt.Println("⚠️  Warnings")
			fmt.Println("===========")
			for _, warn := range warnings {
				fmt.Printf("  • %s\n", warn)
			}
			fmt.Println()
		}
	}

	// Detailed persona/platform view
	if statusVerbose {
		fmt.Println("👤 Personas & Platforms")
		fmt.Println("=======================")
		fmt.Println()

		for _, persona := range cfg.Personas {
			fmt.Printf("📋 %s <%s>\n", persona.Name, persona.Email)
			for _, platform := range persona.Platforms {
				platformLabel := string(platform.Type)
				if platform.BaseURL != "" {
					platformLabel = fmt.Sprintf("%s (%s)", platform.Type, platform.BaseURL)
				}
				fmt.Printf("  └─ %s @ %s\n", platformLabel, platform.Account)

				for _, key := range platform.Keys {
					status := getKeyStatusIcon(key.Status)
					age := ""
					if !key.CreatedAt.IsZero() {
						daysSinceCreation := int(time.Since(key.CreatedAt).Hours() / 24)
						age = fmt.Sprintf(" (age: %dd)", daysSinceCreation)
					}
					fmt.Printf("     └─ %s %s%s\n", status, key.Fingerprint, age)
				}
			}
			fmt.Println()
		}
	}

	// Recommendations
	if missingKeyFiles > 0 || expiredKeys > 0 || keysNeedingRotation > 0 {
		fmt.Println("💡 Recommendations")
		fmt.Println("==================")

		if missingKeyFiles > 0 {
			fmt.Println("• Missing key files detected. Run 'git-keys apply' to regenerate keys.")
		}
		if expiredKeys > 0 {
			fmt.Println("• Expired keys found. Run 'git-keys rotate' to rotate them.")
		}
		if keysNeedingRotation > 0 {
			fmt.Println("• Some keys are >90 days old. Consider rotating with 'git-keys rotate'.")
		}
		fmt.Println()
	}

	return nil
}

func getKeyStatusIcon(status config.KeyStatus) string {
	switch status {
	case config.KeyStatusActive:
		return "✓"
	case config.KeyStatusRevoked:
		return "🔄"
	case config.KeyStatusExpired:
		return "⚠️"
	case config.KeyStatusPending:
		return "⏳"
	default:
		return "?"
	}
}
