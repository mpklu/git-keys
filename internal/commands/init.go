package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
	"github.com/kunlu/git-keys/internal/platform"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize git-keys configuration",
	Long: `Initialize git-keys by creating a configuration file.
	
This command will:
  1. Detect your machine's hardware UUID
  2. Create a new .git-keys.yaml configuration file
  3. Guide you through setting up your first persona
  
If a configuration file already exists, this command will fail unless --force is used.`,
	RunE: runInit,
}

var (
	forceInit bool
)

func init() {
	initCmd.Flags().BoolVarP(&forceInit, "force", "f", false, "overwrite existing configuration")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	logger.Info("Initializing git-keys...")

	// Get config path
	configPath := cfgFile
	if configPath == "" {
		configPath = config.GetDefaultConfigPath()
	}

	logger.Debug("Config path: %s", configPath)

	// Check if config already exists
	mgr := config.NewManager(configPath)
	if mgr.Exists() && !forceInit {
		return fmt.Errorf("configuration file already exists at %s\nUse --force to overwrite", configPath)
	}

	// Get platform information
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
		logger.Warn("Failed to get machine name, using 'unknown': %v", err)
		machineName = "unknown"
	}

	osVersion, err := plat.GetOSVersion()
	if err != nil {
		logger.Warn("Failed to get OS version: %v", err)
		osVersion = ""
	}

	logger.Info("Machine ID: %s", machineID)
	logger.Info("Machine Name: %s", machineName)
	logger.Info("OS: %s %s", plat.GetOS(), osVersion)

	// Create default config
	cfg := mgr.CreateDefault(config.Machine{
		ID:        machineID,
		Name:      machineName,
		OS:        plat.GetOS(),
		OSVersion: osVersion,
	})

	// Interactive setup
	fmt.Println("\n=== Git-Keys Setup ===\n")

	// Ask if user wants to add a persona now
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Would you like to add a persona now? (y/n): ")
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "y" || response == "yes" {
		persona, err := promptForPersona(reader)
		if err != nil {
			return fmt.Errorf("failed to create persona: %w", err)
		}
		cfg.Personas = append(cfg.Personas, *persona)
	}

	// Save configuration
	if err := mgr.Save(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("\n✅ Configuration saved to: %s\n", configPath)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review and edit your configuration file")
	fmt.Println("  2. Run 'git-keys plan' to see what changes will be made")
	fmt.Println("  3. Run 'git-keys apply' to generate keys and update SSH config")

	return nil
}

func promptForPersona(reader *bufio.Reader) (*config.Persona, error) {
	persona := &config.Persona{}

	fmt.Print("\nPersona name (e.g., personal, work): ")
	name, _ := reader.ReadString('\n')
	persona.Name = strings.TrimSpace(name)

	fmt.Print("Email (for git commits): ")
	email, _ := reader.ReadString('\n')
	persona.Email = strings.TrimSpace(email)

	// Ask for platform
	fmt.Print("\nAdd a platform? (y/n): ")
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "y" || response == "yes" {
		platform, err := promptForPlatform(reader)
		if err != nil {
			return nil, err
		}
		persona.Platforms = append(persona.Platforms, *platform)
	}

	return persona, nil
}

func promptForPlatform(reader *bufio.Reader) (*config.Platform, error) {
	platform := &config.Platform{}

	fmt.Print("Platform type (github/gitlab): ")
	platformType, _ := reader.ReadString('\n')
	platformType = strings.TrimSpace(strings.ToLower(platformType))

	switch platformType {
	case "github":
		platform.Type = config.PlatformGitHub
	case "gitlab":
		platform.Type = config.PlatformGitLab
	default:
		return nil, fmt.Errorf("invalid platform type: %s", platformType)
	}

	fmt.Print("Account/username: ")
	account, _ := reader.ReadString('\n')
	platform.Account = strings.TrimSpace(account)

	if platform.Type == config.PlatformGitLab {
		fmt.Print("GitLab base URL (press Enter for gitlab.com): ")
		baseURL, _ := reader.ReadString('\n')
		baseURL = strings.TrimSpace(baseURL)
		if baseURL == "" {
			baseURL = "https://gitlab.com"
		}
		platform.BaseURL = baseURL
	}

	return platform, nil
}
