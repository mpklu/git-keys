package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
	"github.com/spf13/cobra"
)

var (
	restoreForce bool
)

var restoreCmd = &cobra.Command{
	Use:   "restore [backup-file]",
	Short: "Restore configuration from a backup",
	Long: `Restore git-keys configuration from a backup file.

Backups are created automatically by the rebuild command and stored in:
  ~/.git-keys/backups/backup-YYYY-MM-DD-HHMMSS.json

This command will restore:
  • git-keys configuration file (~/.git-keys.yaml)
  • Overview of what was backed up (for manual key recreation)

This command will NOT restore:
  • SSH keys (must be regenerated with 'git-keys apply')
  • SSH config blocks (will be created by 'git-keys apply')
  • Remote keys (will be recreated by 'git-keys apply')

After restoring, run 'git-keys apply' to regenerate keys and apply configuration.

Examples:
  # List available backups
  git-keys restore

  # Restore from specific backup
  git-keys restore ~/.git-keys/backups/backup-2024-01-15-143022.json

  # Force restore without confirmation
  git-keys restore backup.json --force
`,
	RunE: runRestore,
}

func init() {
	restoreCmd.Flags().BoolVarP(&restoreForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(restoreCmd)
}

func runRestore(cmd *cobra.Command, args []string) error {
	backupDir := filepath.Join(os.Getenv("HOME"), ".git-keys", "backups")

	// If no backup file specified, list available backups
	if len(args) == 0 {
		return listBackups(backupDir)
	}

	backupPath := args[0]

	// If relative path, assume it's in the backup directory
	if !filepath.IsAbs(backupPath) {
		backupPath = filepath.Join(backupDir, backupPath)
	}

	// Read backup file
	backupData, err := readBackupFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Show backup summary
	fmt.Println("\n📦 Backup Information")
	fmt.Println("====================")
	fmt.Printf("Created: %s\n", backupData.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("File: %s\n\n", backupPath)

	if backupData.OldConfig != nil {
		fmt.Printf("Personas: %d\n", len(backupData.OldConfig.Personas))
		totalPlatforms := 0
		totalKeys := 0
		for _, persona := range backupData.OldConfig.Personas {
			totalPlatforms += len(persona.Platforms)
			for _, platform := range persona.Platforms {
				totalKeys += len(platform.Keys)
			}
		}
		fmt.Printf("Platforms: %d\n", totalPlatforms)
		fmt.Printf("Keys: %d\n", totalKeys)
	}

	if backupData.ScanResult != nil {
		fmt.Printf("\nScanned at restore time:\n")
		fmt.Printf("  SSH keys: %d\n", len(backupData.ScanResult.Keys))
		fmt.Printf("  SSH hosts: %d\n", len(backupData.ScanResult.SSHConfigHosts))
	}

	// Check if config already exists
	configPath := config.GetDefaultConfigPath()
	configExists := false
	if _, err := os.Stat(configPath); err == nil {
		configExists = true
	}

	if configExists && !restoreForce {
		fmt.Printf("\n⚠️  Warning: Configuration file already exists at:\n   %s\n\n", configPath)
		fmt.Print("Overwrite existing configuration? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "yes" {
			fmt.Println("\n❌ Restore cancelled.")
			return nil
		}
	}

	// Restore configuration
	fmt.Println("\n🔄 Restoring configuration...")

	if backupData.OldConfig != nil {
		// Save config to file
		configMgr := config.NewManager(configPath)
		if err := configMgr.Save(backupData.OldConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("✓ Configuration restored to: %s\n", configPath)
	} else {
		fmt.Println("⚠️  No configuration in backup to restore")
	}

	// Show next steps
	fmt.Println("\n✅ Restore Complete")
	fmt.Println("===================")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the restored configuration:")
	fmt.Printf("     cat %s\n", configPath)
	fmt.Println("\n  2. Generate keys and apply configuration:")
	fmt.Println("     git-keys apply")
	fmt.Println("\n  3. Verify SSH config:")
	fmt.Println("     cat ~/.ssh/config")
	fmt.Println()

	return nil
}

func listBackups(backupDir string) error {
	fmt.Println("\n📦 Available Backups")
	fmt.Println("===================")
	fmt.Println()

	// Check if backup directory exists
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		fmt.Printf("No backups found. Backup directory does not exist:\n  %s\n\n", backupDir)
		fmt.Println("Backups are created automatically when you run:")
		fmt.Println("  git-keys rebuild")
		fmt.Println()
		return nil
	}

	// Read backup files
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	backups := []backupInfo{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		backupPath := filepath.Join(backupDir, entry.Name())
		info, err := os.Stat(backupPath)
		if err != nil {
			continue
		}

		// Try to read backup to get more info
		backupData, err := readBackupFile(backupPath)
		if err != nil {
			logger.Warn("Failed to read backup %s: %v", entry.Name(), err)
			continue
		}

		backups = append(backups, backupInfo{
			Filename:  entry.Name(),
			Path:      backupPath,
			Size:      info.Size(),
			Timestamp: backupData.Timestamp,
			Personas:  countPersonas(backupData),
		})
	}

	if len(backups) == 0 {
		fmt.Printf("No backups found in:\n  %s\n\n", backupDir)
		fmt.Println("Backups are created automatically when you run:")
		fmt.Println("  git-keys rebuild")
		fmt.Println()
		return nil
	}

	// Display backups
	for i, backup := range backups {
		fmt.Printf("%d. %s\n", i+1, backup.Filename)
		fmt.Printf("   Created: %s\n", backup.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("   Size: %s\n", formatBytes(backup.Size))
		if backup.Personas > 0 {
			fmt.Printf("   Personas: %d\n", backup.Personas)
		}
		fmt.Println()
	}

	fmt.Println("To restore a backup:")
	fmt.Printf("  git-keys restore %s\n", backups[0].Filename)
	fmt.Println()

	return nil
}

func readBackupFile(path string) (*BackupData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var backup BackupData
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil, fmt.Errorf("failed to parse backup: %w", err)
	}

	return &backup, nil
}

func countPersonas(backup *BackupData) int {
	if backup.OldConfig != nil {
		return len(backup.OldConfig.Personas)
	}
	return 0
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

type backupInfo struct {
	Filename  string
	Path      string
	Size      int64
	Timestamp time.Time
	Personas  int
}
