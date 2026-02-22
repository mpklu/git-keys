package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kevinburke/ssh_config"
	"github.com/kunlu/git-keys/internal/api"
	"github.com/kunlu/git-keys/internal/config"
	"github.com/kunlu/git-keys/internal/logger"
	"github.com/kunlu/git-keys/internal/sshkey"
	"github.com/spf13/cobra"
)

var (
	scanPath        string
	scanCheckRemote bool
	scanJSON        bool
)

// DiscoveredKey represents a found SSH key
type DiscoveredKey struct {
	Path        string
	Type        string
	Bits        int
	Fingerprint string
	Comment     string
	Created     time.Time
	UsedBy      []string // SSH config hosts using this key
	InAgent     bool
	OnGitHub    bool
	OnGitLab    bool
}

// ScanResult holds all discovered information
type ScanResult struct {
	Keys           []DiscoveredKey
	SSHConfigHosts []SSHConfigHost
	GitConfig      GitConfig
}

type SSHConfigHost struct {
	Host         string
	HostName     string
	IdentityFile string
	User         string
}

type GitConfig struct {
	GlobalName  string
	GlobalEmail string
	Includes    []GitInclude
}

type GitInclude struct {
	Path                string               `json:"path"`
	Condition           string               `json:"condition"`
	Name                string               `json:"name"`
	Email               string               `json:"email"`
	DiscoveredPlatforms []DiscoveredPlatform `json:"discovered_platforms,omitempty"`
}

type DiscoveredPlatform struct {
	Type      string   `json:"type"`               // "github" or "gitlab"
	BaseURL   string   `json:"base_url,omitempty"` // For self-hosted GitLab
	RepoCount int      `json:"repo_count"`         // Number of repos found for this platform
	Groups    []string `json:"groups,omitempty"`   // Group/namespace names (for reference, not auth)
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Discover existing SSH keys and configuration",
	Long: `Non-destructive discovery of existing SSH keys, configs, and Git identity.

Scans:
  - SSH keys in ~/.ssh/ (discovers all id_* and custom key files)
  - SSH config entries (host mappings, identity files)
  - Git global config (user.name, user.email)
  - Active keys in SSH agent (if running)
  - Remote platform keys (if --check-remote and credentials available)

This helps you understand your current setup before migration.`,
	RunE: runScan,
}

func init() {
	scanCmd.Flags().StringVar(&scanPath, "path", filepath.Join(os.Getenv("HOME"), ".ssh"), "SSH directory to scan")
	scanCmd.Flags().BoolVar(&scanCheckRemote, "check-remote", false, "Query GitHub/GitLab for registered keys (requires tokens)")
	scanCmd.Flags().BoolVar(&scanJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	logger.Info("Scanning SSH configuration...")

	result := &ScanResult{}

	// Scan for SSH keys
	keys, err := scanSSHKeys(scanPath)
	if err != nil {
		logger.Warn("Failed to scan SSH keys: %v", err)
	} else {
		result.Keys = keys
	}

	// Parse SSH config
	hosts, err := scanSSHConfig(scanPath)
	if err != nil {
		logger.Warn("Failed to parse SSH config: %v", err)
	} else {
		result.SSHConfigHosts = hosts
	}

	// Match keys to hosts
	matchKeysToHosts(result)

	// Check SSH agent
	checkSSHAgent(result)

	// Parse Git config
	gitConf, err := scanGitConfig()
	if err != nil {
		logger.Warn("Failed to parse Git config: %v", err)
	} else {
		result.GitConfig = gitConf
	}

	// Check remote platforms if requested
	if scanCheckRemote {
		if err := checkRemotePlatforms(result); err != nil {
			logger.Warn("Failed to check remote platforms: %v", err)
		}
	}

	// Output results
	if scanJSON {
		return outputJSON(result)
	}

	return outputHuman(result)
}

func scanSSHKeys(sshDir string) ([]DiscoveredKey, error) {
	var keys []DiscoveredKey

	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return nil, fmt.Errorf("reading SSH directory: %w", err)
	}

	keyMgr := sshkey.NewManager(sshDir)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Look for SSH key files (private keys typically don't have extensions)
		// Common patterns: id_rsa, id_ed25519, id_ecdsa, or custom names
		// Skip .pub files, known_hosts, config, etc.
		if strings.HasSuffix(name, ".pub") ||
			name == "known_hosts" ||
			name == "known_hosts.old" ||
			name == "config" ||
			name == "authorized_keys" ||
			strings.HasPrefix(name, ".") {
			continue
		}

		keyPath := filepath.Join(sshDir, name)

		// Check if there's a corresponding .pub file
		pubPath := keyPath + ".pub"
		if _, err := os.Stat(pubPath); err != nil {
			// No .pub file, probably not a key pair
			continue
		}

		// Try to get key information
		// Pass just the filename since Manager already knows the keysDir
		fingerprint, err := keyMgr.GetFingerprint(name)
		if err != nil {
			// Not a valid SSH key
			logger.Debug("Failed to get fingerprint for %s: %v", name, err)
			continue
		}

		// Get public key info
		pubKey, err := keyMgr.GetPublicKey(name)
		if err != nil {
			logger.Debug("Failed to read public key %s: %v", name, err)
			continue
		}

		// Parse key type and comment from public key
		parts := strings.Fields(pubKey)
		keyType := "unknown"
		comment := ""
		if len(parts) >= 1 {
			keyType = parts[0]
		}
		if len(parts) >= 3 {
			comment = strings.Join(parts[2:], " ")
		}

		// Determine bit size
		bits := getKeyBits(keyType, keyPath)

		// Get file modification time as proxy for creation time
		info, _ := os.Stat(keyPath)
		created := info.ModTime()

		key := DiscoveredKey{
			Path:        keyPath,
			Type:        keyType,
			Bits:        bits,
			Fingerprint: fingerprint,
			Comment:     comment,
			Created:     created,
			UsedBy:      []string{},
		}

		keys = append(keys, key)
	}

	// Sort by creation time (newest first)
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Created.After(keys[j].Created)
	})

	return keys, nil
}

func getKeyBits(keyType, keyPath string) int {
	// For ed25519, it's always 256 bits
	if strings.Contains(keyType, "ed25519") {
		return 256
	}

	// For RSA/ECDSA, try to get actual bit size
	cmd := exec.Command("ssh-keygen", "-l", "-f", keyPath)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	// Output format: "2048 SHA256:... comment (RSA)"
	fields := strings.Fields(string(output))
	if len(fields) >= 1 {
		var bits int
		fmt.Sscanf(fields[0], "%d", &bits)
		return bits
	}

	return 0
}

func scanSSHConfig(sshDir string) ([]SSHConfigHost, error) {
	configPath := filepath.Join(sshDir, "config")

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []SSHConfigHost{}, nil
		}
		return nil, err
	}
	defer file.Close()

	cfg, err := ssh_config.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("parsing SSH config: %w", err)
	}

	var hosts []SSHConfigHost

	for _, host := range cfg.Hosts {
		// Skip wildcard and managed blocks
		if len(host.Patterns) == 0 {
			continue
		}

		pattern := host.Patterns[0].String()
		if pattern == "*" {
			continue
		}

		// Check if this is inside a git-keys managed block by looking at previous comments
		// (This is a simple check - a more robust version would parse comments)

		hostEntry := SSHConfigHost{
			Host: pattern,
		}

		// Extract HostName
		if hostname, err := cfg.Get(pattern, "HostName"); err == nil {
			hostEntry.HostName = hostname
		}

		// Extract IdentityFile
		if identityFile, err := cfg.Get(pattern, "IdentityFile"); err == nil {
			// Expand ~ to home directory
			if strings.HasPrefix(identityFile, "~") {
				identityFile = strings.Replace(identityFile, "~", os.Getenv("HOME"), 1)
			}
			hostEntry.IdentityFile = identityFile
		}

		// Extract User
		if user, err := cfg.Get(pattern, "User"); err == nil {
			hostEntry.User = user
		}

		if hostEntry.IdentityFile != "" {
			hosts = append(hosts, hostEntry)
		}
	}

	return hosts, nil
}

func matchKeysToHosts(result *ScanResult) {
	for i := range result.Keys {
		key := &result.Keys[i]
		for _, host := range result.SSHConfigHosts {
			// Check if this host uses this key
			if host.IdentityFile == key.Path || host.IdentityFile == key.Path+".pub" {
				key.UsedBy = append(key.UsedBy, host.Host)
			}
		}
	}
}

func checkSSHAgent(result *ScanResult) {
	// Run ssh-add -l to list keys in agent
	cmd := exec.Command("ssh-add", "-l")
	output, err := cmd.Output()
	if err != nil {
		// Agent not running or no keys
		return
	}

	lines := strings.Split(string(output), "\n")
	agentFingerprints := make(map[string]bool)

	for _, line := range lines {
		if line == "" {
			continue
		}
		// Format: "2048 SHA256:... comment (RSA)"
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			agentFingerprints[fields[1]] = true
		}
	}

	// Match agent fingerprints to discovered keys
	for i := range result.Keys {
		key := &result.Keys[i]
		if agentFingerprints[key.Fingerprint] {
			key.InAgent = true
		}
	}
}

func scanGitConfig() (GitConfig, error) {
	gitConf := GitConfig{}

	// Get global user.name
	cmd := exec.Command("git", "config", "--global", "user.name")
	if output, err := cmd.Output(); err == nil {
		gitConf.GlobalName = strings.TrimSpace(string(output))
	}

	// Get global user.email
	cmd = exec.Command("git", "config", "--global", "user.email")
	if output, err := cmd.Output(); err == nil {
		gitConf.GlobalEmail = strings.TrimSpace(string(output))
	}

	// Try to find conditional includes
	homeDir := os.Getenv("HOME")
	globalConfigPath := filepath.Join(homeDir, ".gitconfig")

	data, err := os.ReadFile(globalConfigPath)
	if err != nil {
		return gitConf, nil // Not an error if .gitconfig doesn't exist
	}

	// Simple regex to find includeIf sections
	includePattern := regexp.MustCompile(`\[includeIf "gitdir:([^"]+)"\]\s+path\s*=\s*(.+)`)
	matches := includePattern.FindAllStringSubmatch(string(data), -1)

	for _, match := range matches {
		if len(match) >= 3 {
			gitdir := strings.TrimSpace(match[1])
			path := strings.TrimSpace(match[2])

			// Try to read the included file
			if strings.HasPrefix(path, "~") {
				path = strings.Replace(path, "~", homeDir, 1)
			}

			var name, email string
			if includeData, err := os.ReadFile(path); err == nil {
				content := string(includeData)
				if nameMatch := regexp.MustCompile(`name\s*=\s*(.+)`).FindStringSubmatch(content); len(nameMatch) >= 2 {
					name = strings.TrimSpace(nameMatch[1])
				}
				if emailMatch := regexp.MustCompile(`email\s*=\s*(.+)`).FindStringSubmatch(content); len(emailMatch) >= 2 {
					email = strings.TrimSpace(emailMatch[1])
				}
			}

			gitConf.Includes = append(gitConf.Includes, GitInclude{
				Path:                path,
				Condition:           gitdir,
				Name:                name,
				Email:               email,
				DiscoveredPlatforms: discoverPlatformsInDirectory(gitdir),
			})
		}
	}

	return gitConf, nil
}

// discoverPlatformsInDirectory scans git repos in a directory to discover platforms
func discoverPlatformsInDirectory(gitdir string) []DiscoveredPlatform {
	// Expand path
	homeDir := os.Getenv("HOME")
	if strings.HasPrefix(gitdir, "~") {
		gitdir = strings.Replace(gitdir, "~", homeDir, 1)
	}
	// Remove trailing slash and gitdir: prefix
	gitdir = strings.TrimSuffix(gitdir, "/")
	gitdir = strings.TrimPrefix(gitdir, "gitdir:")

	// Check if directory exists
	if _, err := os.Stat(gitdir); os.IsNotExist(err) {
		return nil
	}

	platformMap := make(map[string]*DiscoveredPlatform) // key: "platform:account:baseurl"

	// Walk subdirectories (max 2 levels deep)
	err := filepath.Walk(gitdir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return err
		}

		// Skip if we're too deep
		relPath, _ := filepath.Rel(gitdir, path)
		depth := len(strings.Split(relPath, string(os.PathSeparator)))
		if depth > 2 {
			return filepath.SkipDir
		}

		// Check if this is a git repo
		gitConfigPath := filepath.Join(path, ".git", "config")
		if _, err := os.Stat(gitConfigPath); os.IsNotExist(err) {
			return nil
		}

		// Read git config
		data, err := os.ReadFile(gitConfigPath)
		if err != nil {
			return nil
		}

		// Parse remote URLs
		remotePattern := regexp.MustCompile(`\[remote\s+"[^"]*"\]\s+url\s*=\s*(.+)`)
		matches := remotePattern.FindAllStringSubmatch(string(data), -1)

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			url := strings.TrimSpace(match[1])
			platformType, baseURL, group := parseGitRemoteURL(url)
			if platformType == "" {
				continue
			}

			// Create key for deduplication by platform type and base URL only
			key := fmt.Sprintf("%s:%s", platformType, baseURL)
			if existing, exists := platformMap[key]; exists {
				existing.RepoCount++
				// Add group if not already present
				if group != "" && !contains(existing.Groups, group) {
					existing.Groups = append(existing.Groups, group)
				}
			} else {
				platform := &DiscoveredPlatform{
					Type:      platformType,
					BaseURL:   baseURL,
					RepoCount: 1,
					Groups:    []string{},
				}
				if group != "" {
					platform.Groups = append(platform.Groups, group)
				}
				platformMap[key] = platform
			}
		}

		return nil
	})

	if err != nil {
		logger.Debug("Error walking directory %s: %v", gitdir, err)
	}

	// Convert map to slice
	var platforms []DiscoveredPlatform
	for _, p := range platformMap {
		platforms = append(platforms, *p)
	}

	return platforms
}

// contains checks if a string is in a slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// parseGitRemoteURL extracts platform info from a git remote URL
// Returns: platformType, baseURL, group/namespace
func parseGitRemoteURL(url string) (string, string, string) {
	url = strings.TrimSpace(url)

	var hostname, group string

	// Handle SSH URLs: git@github.com:username/repo.git or git@gitlab.com:group/repo.git
	if strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://") {
		// Remove protocol
		url = strings.TrimPrefix(url, "ssh://")
		url = strings.TrimPrefix(url, "git@")

		// Split on : or /
		if strings.Contains(url, ":") {
			parts := strings.Split(url, ":")
			if len(parts) >= 2 {
				hostname = parts[0]
				// Parse group/namespace from path: group/repo.git
				pathParts := strings.Split(parts[1], "/")
				if len(pathParts) >= 1 {
					group = pathParts[0]
				}
			}
		}
	} else if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		// Handle HTTPS URLs: https://github.com/username/repo.git
		url = strings.TrimPrefix(url, "https://")
		url = strings.TrimPrefix(url, "http://")

		parts := strings.Split(url, "/")
		if len(parts) >= 2 {
			hostname = parts[0]
			group = parts[1]
		}
	} else {
		return "", "", ""
	}

	if hostname == "" {
		return "", "", ""
	}

	// Determine platform type and base URL
	var platformType, baseURL string

	if strings.Contains(hostname, "github.com") {
		platformType = "github"
		baseURL = "" // Empty for github.com
	} else if strings.Contains(hostname, "gitlab") {
		platformType = "gitlab"
		if hostname != "gitlab.com" {
			baseURL = "https://" + hostname
		} else {
			baseURL = "" // Empty for gitlab.com
		}
	} else {
		// Unknown platform, skip
		return "", "", ""
	}

	return platformType, baseURL, group
}

func checkRemotePlatforms(result *ScanResult) error {
	ctx := context.Background()

	// Try to get GitHub token
	githubTokenMgr := api.NewTokenManager("git-keys-github")

	githubToken, err := githubTokenMgr.GetToken("default")
	if err == nil && githubToken != "" {
		logger.Info("Checking GitHub for registered keys...")
		client := api.NewGitHubClient(githubToken)
		remoteKeys, err := client.ListKeys(ctx)
		if err != nil {
			logger.Warn("Failed to list GitHub keys: %v", err)
		} else {
			matchRemoteKeys(result, remoteKeys, "GitHub")
		}
	}

	// Try to get GitLab token
	gitlabTokenMgr := api.NewTokenManager("git-keys-gitlab")
	gitlabToken, err := gitlabTokenMgr.GetToken("default")
	if err == nil && gitlabToken != "" {
		logger.Info("Checking GitLab for registered keys...")

		// Try to load config to get GitLab base URL
		configPath := config.GetDefaultConfigPath()
		mgr := config.NewManager(configPath)
		baseURL := "https://gitlab.com"
		if mgr.Exists() {
			if cfg, err := mgr.Load(); err == nil {
				for _, persona := range cfg.Personas {
					for _, platform := range persona.Platforms {
						if platform.Type == config.PlatformGitLab && platform.BaseURL != "" {
							baseURL = platform.BaseURL
							break
						}
					}
				}
			}
		}

		client := api.NewGitLabClient(baseURL, gitlabToken)
		remoteKeys, err := client.ListKeys(ctx)
		if err != nil {
			logger.Warn("Failed to list GitLab keys: %v", err)
		} else {
			matchRemoteKeys(result, remoteKeys, "GitLab")
		}
	}

	return nil
}

func matchRemoteKeys(result *ScanResult, remoteKeys []api.SSHKey, platform string) {
	for i := range result.Keys {
		key := &result.Keys[i]
		for _, remote := range remoteKeys {
			// Compare fingerprints (strip "SHA256:" prefix if present)
			localFP := strings.TrimPrefix(key.Fingerprint, "SHA256:")
			remoteFP := strings.TrimPrefix(remote.Fingerprint, "SHA256:")

			if localFP == remoteFP {
				if platform == "GitHub" {
					key.OnGitHub = true
				} else if platform == "GitLab" {
					key.OnGitLab = true
				}
				break
			}
		}
	}
}

func outputHuman(result *ScanResult) error {
	fmt.Println()
	fmt.Println("🔍 SSH Configuration Scan Results")
	fmt.Println("==================================")
	fmt.Println()

	// SSH Keys
	if len(result.Keys) == 0 {
		fmt.Println("No SSH keys found in", scanPath)
	} else {
		fmt.Printf("Found %d SSH key(s):\n\n", len(result.Keys))

		for _, key := range result.Keys {
			status := "✓"
			if len(key.UsedBy) == 0 && !key.InAgent {
				status = "⚠"
			}

			fmt.Printf("  %s %s (%s, %d bits)\n", status, filepath.Base(key.Path), key.Type, key.Bits)
			fmt.Printf("    Fingerprint: %s\n", key.Fingerprint)
			if key.Comment != "" {
				fmt.Printf("    Comment: %s\n", key.Comment)
			}
			fmt.Printf("    Created: %s\n", key.Created.Format("2006-01-02"))

			if len(key.UsedBy) > 0 {
				fmt.Printf("    Used by: %s\n", strings.Join(key.UsedBy, ", "))
			}

			if key.InAgent {
				fmt.Println("    In SSH agent: yes")
			}

			if scanCheckRemote {
				var platforms []string
				if key.OnGitHub {
					platforms = append(platforms, "GitHub")
				}
				if key.OnGitLab {
					platforms = append(platforms, "GitLab")
				}
				if len(platforms) > 0 {
					fmt.Printf("    Remote: Found on %s\n", strings.Join(platforms, ", "))
				} else {
					fmt.Println("    Remote: Not found on any platform")
				}
			}

			if len(key.UsedBy) == 0 && !key.InAgent {
				fmt.Println("    ⚠ Not referenced in SSH config or agent")
				fmt.Println("    Recommendation: Archive or delete")
			}

			fmt.Println()
		}
	}

	// SSH Config
	if len(result.SSHConfigHosts) > 0 {
		fmt.Println("SSH Config Entries:")
		fmt.Println()

		for _, host := range result.SSHConfigHosts {
			fmt.Printf("  Host %s\n", host.Host)
			if host.HostName != "" && host.HostName != host.Host {
				fmt.Printf("    HostName %s\n", host.HostName)
			}
			if host.User != "" {
				fmt.Printf("    User %s\n", host.User)
			}
			fmt.Printf("    IdentityFile %s\n", host.IdentityFile)
			fmt.Println()
		}
	}

	// Git Config
	if result.GitConfig.GlobalName != "" || result.GitConfig.GlobalEmail != "" {
		fmt.Println("Git Identity:")
		fmt.Println()
		fmt.Printf("  Global: %s <%s>\n", result.GitConfig.GlobalName, result.GitConfig.GlobalEmail)
		fmt.Println()

		if len(result.GitConfig.Includes) > 0 {
			fmt.Println("  Conditional Includes:")
			for _, inc := range result.GitConfig.Includes {
				fmt.Printf("    %s → %s <%s>\n", inc.Condition, inc.Name, inc.Email)
			}
			fmt.Println()
		}
	}

	// Recommendations
	fmt.Println("Recommendation:")
	fmt.Println("  Run: git-keys import --interactive")
	fmt.Println("  This will help you adopt existing keys into git-keys management.")
	fmt.Println()

	return nil
}

func outputJSON(result *ScanResult) error {
	// For now, just print a simple message
	// In a full implementation, this would marshal result to JSON
	fmt.Println("JSON output not yet implemented")
	return nil
}
