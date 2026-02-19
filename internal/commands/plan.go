package commands

import (
	"fmt"

	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show what changes git-keys will make",
	Long:  `Show a detailed plan of changes without applying them.`,
	RunE:  runPlan,
}

func init() {
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	logger.Info("Generating execution plan...")

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

	// Display summary
	fmt.Printf("\n📋 Configuration Summary:\n\n")
	fmt.Printf("Machine: %s (%s)\n", cfg.Machine.Name, cfg.Machine.ID)
	fmt.Printf("Personas: %d\n", len(cfg.Personas))

	for _, persona := range cfg.Personas {
		fmt.Printf("\n  • %s (%s)\n", persona.Name, persona.Email)
		for _, platform := range persona.Platforms {
			fmt.Printf("    - %s/%s\n", platform.Type, platform.Account)
			activeKey := platform.GetActiveKey()
			if activeKey != nil {
				fmt.Printf("      Key: %s (expires: %s)\n",
					activeKey.Fingerprint,
					activeKey.ExpiresAt.Format("2006-01-02"))
			} else {
				fmt.Printf("      ⚠️  No active key - run 'git-keys apply' to create\n")
			}
		}
	}

	fmt.Println("\nRun 'git-keys apply' to apply configuration.")
	return nil
}
