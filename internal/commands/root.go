package commands

import (
	"fmt"
	"os"

	"github.com/kunlu/git-keys/internal/logger"
	"github.com/spf13/cobra"
)

var (
	cfgFile  string
	logLevel string
	rootCmd  = &cobra.Command{
		Use:   "git-keys",
		Short: "Automated SSH key management for Git platforms",
		Long: `git-keys is a tool for managing SSH keys across GitHub and GitLab.
It automatically generates, rotates, and manages SSH keys with per-persona
configuration, ensuring secure and organized access to your repositories.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Set up logging
			if logLevel != "" {
				if err := logger.SetLevelFromString(logLevel); err != nil {
					fmt.Fprintf(os.Stderr, "Invalid log level: %s\n", logLevel)
					os.Exit(1)
				}
			}
		},
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.git-keys.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (error, warn, info, debug, trace)")
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

// GetConfigFile returns the config file path
func GetConfigFile() string {
	return cfgFile
}
