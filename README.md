# git-keys

> Automated SSH key management for Git platforms (GitHub & GitLab)

`git-keys` is a command-line tool that simplifies SSH key management across GitHub and GitLab. It supports multiple personas (personal, work, etc.), automatic key generation, SSH config management, and follows security best practices.

## Features

- **Multi-Persona Support**: Separate SSH keys for personal, work, and other identities
- **Platform Support**: GitHub and self-hosted GitLab
- **Automated Key Generation**: Creates ed25519 or RSA keys with proper permissions
- **Automatic Key Upload**: Upload keys to GitHub/GitLab automatically via API
- **SSH Config Management**: Automatically updates `~/.ssh/config` with managed blocks
- **Git Identity Management**: Automatic identity switching based on directory
- **macOS Keychain Integration**: Easy SSH key management with Keychain storage
- **Secure Token Storage**: API tokens stored in `.env` file (gitignored)
- **Key Lifecycle Management**: Creation, rotation, revocation, and expiration tracking
- **Configuration Validation**: Built-in validation to catch configuration errors
- **Health Monitoring**: Status checks for key age, permissions, and file integrity
- **Backup & Restore**: Automatic backups with easy restoration
- **Machine-Specific**: Keys are tied to your machine's hardware UUID

## Installation

### From Source

```bash
cd git-keys
go build -o git-keys ./cmd/git-keys
sudo mv git-keys /usr/local/bin/
```

### Keychain Setup (macOS)

**Adding SSH keys to Keychain:**

```bash
# Add all git-keys to Keychain and SSH agent
git-keys keychain add --all

# Or interactively
git-keys keychain add
```

See the [`git-keys keychain`](#git-keys-keychain-add) command for more details.

**Removing invalid tokens from Keychain:**

```bash
# Remove GitLab token
security delete-generic-password -s "gitlab-api-test" -a "git-keys"

# Verify it's removed (should fail)
security find-generic-password -s "gitlab-api-test" -a "git-keys" -w
```

## Quick Start

### Choosing the Right Workflow

**Decision Tree:**
```
Do you have existing SSH keys?
├─ No → Use `git-keys init`
└─ Yes → Do you want to keep your current keys?
    ├─ Yes → Use `git-keys import`
    └─ No → Use `git-keys rebuild --interactive`
```

### 1. For Fresh Start: `init` Workflow

When starting from scratch with no existing SSH keys:

```bash
# Initialize configuration
git-keys init

# Edit the generated config file
vim ~/.git-keys.yaml

# Preview changes
git-keys plan

# Apply configuration
git-keys apply
```

### 2. For Migration: `import` Workflow

When you have existing SSH keys you want to preserve:

```bash
# Discover your existing setup
git-keys scan

# Import keys interactively
git-keys import

# Preview what will change
git-keys plan

# Apply configuration
git-keys apply
```

### 3. For Clean Rebuild: `rebuild` Workflow

When you want to start fresh but preserve knowledge of your old setup:

```bash
# Interactive rebuild with backup, cleanup, and guided setup
git-keys rebuild --interactive

# Or preview without making changes
git-keys rebuild --dry-run
```

This will:
1. 🔍 Scan your current SSH keys, config, and git identities
2. 💾 Create timestamped backup in `~/.git-keys/backups/`
3. 📋 Show summary of discovered setup
4. ⚠️  Confirm cleanup operation
5. 🧹 Clean up (revoke remote keys, remove config blocks, delete files)
6. 🎯 Guide you through recreating your setup
7. ✅ Generate new keys and apply configuration

## Commands Reference

### Discovery & Migration

#### `git-keys scan`

Discover existing SSH keys and configuration.

```bash
# Basic scan
git-keys scan

# Check which keys are registered on platforms
git-keys scan --check-remote

# Output as JSON
git-keys scan --json

# Scan specific directory
git-keys scan --path ~/custom-ssh-dir
```

Discovers:
- SSH keys in `~/.ssh/`
- SSH config entries
- Git identity configuration
- Keys loaded in SSH agent
- Remote keys (with `--check-remote`)

#### `git-keys import`

Import existing SSH keys into git-keys management.

```bash
# Interactive import wizard
git-keys import

# Preview what would be imported
git-keys import --dry-run
```

The wizard will:
- Map existing keys to personas
- Optionally reorganize keys
- Update SSH config with managed blocks
- Create git-keys configuration

All changes are backed up and reversible.

### Setup & Configuration

#### `git-keys init`

Initialize git-keys configuration.

```bash
git-keys init
```

Creates `~/.git-keys.yaml` and guides you through:
- Machine hardware UUID detection
- First persona setup
- Platform configuration

#### `git-keys setup-git`

Configure or reconfigure git identity and SSH settings for platforms.

```bash
# Setup git config for all platforms
git-keys setup-git

# Preview what would be created
git-keys setup-git --dry-run
```

This will:
- Create platform-specific git config files (e.g., `~/.gitconfig-personal-github-myusername`)
- Add conditional `includeIf` entries to `~/.gitconfig`
- Configure git user name/email from persona
- Set up SSH URL rewrites for each platform's keys

After running this, your git commits will automatically use the correct identity and SSH key based on which directory you're working in.

**Note:** This is typically done automatically by `git-keys apply`. Use this command to reconfigure directory patterns without regenerating keys.

**Example workflow:**
```bash
# Setup personas and apply (automatically sets up git config)
git-keys init
git-keys apply
# During apply, you'll be prompted: Enter directory pattern for each platform
#   gelileo <personal@example.com> - github/myusername: ~/Projects/myusername/
#   work <work@company.com> - gitlab/workuser: ~/Projects/work/

# Now commits automatically use correct identity
cd ~/Projects/myusername/myrepo
git commit -m "test"  # Uses personal email & myusername's SSH key

cd ~/Projects/work/project
git commit -m "test"  # Uses work email & workuser's SSH key

# To reconfigure directory patterns later:
git-keys setup-git
```

#### `git-keys plan`

Preview what changes will be made.

```bash
git-keys plan
```

Shows:
- Keys to be generated
- SSH config changes
- Platform accounts to be configured

#### `git-keys apply`

Generate keys, upload to platforms, and configure git identity switching.

```bash
# Apply with confirmation prompt
git-keys apply

# Skip confirmation
git-keys apply -y
```

This will:
- Generate SSH keys for each persona/platform
- Update your SSH config with managed blocks
- **Automatically upload keys to GitHub/GitLab** (if API tokens are configured)
- Prompt for tokens if not found in `.env` file
- Set up git identity switching (prompts for directory patterns if not in config)
- Fall back to manual upload instructions if tokens unavailable

**Automatic Upload Setup:**

Create a `.env` file in the git-keys project directory:

```bash
# .env format
GITHUB_API_TOKEN_username=ghp_your_token_here
GITLAB_TOKEN_username=glpat_your_token_here
```

The `[username]` must match the account name in your `~/.git-keys.yaml` configuration.

See [`.env.example`](.env.example) for detailed token setup instructions.

### SSH Agent & Keychain Management

#### `git-keys keychain add`

Add SSH keys to macOS Keychain and SSH agent.

```bash
# Add all keys without prompts
git-keys keychain add --all

# Interactively add keys (default: yes)
git-keys keychain add
```

This will:
- Add keys to the SSH agent for immediate use
- Store keys in macOS Keychain for persistence across reboots
- Show which keys are already loaded in the agent
- Prompt for confirmation for each key (unless `--all` is used)
- **Test SSH connections to verify setup** (prompts for confirmation)

Keys added to Keychain will be automatically loaded when you first use SSH after a restart.

**Connection Testing:**

After adding keys, git-keys will offer to test your SSH connections:

```bash
git-keys keychain add --all

✅ Summary: 3 added, 0 skipped
Keys have been added to the SSH agent and macOS Keychain.

Test SSH connections to verify setup? [Y/n]: <Enter>

Testing SSH connections...
  ✓ gelileo (github): Hi gelileo!
  ✓ kal (gitlab): Welcome to GitLab, @kal!
  ✓ mpklu (github): Hi mpklu!

✅ All 3 connection(s) successful!
```

This verifies that:
- SSH keys are correctly loaded in the agent
- SSH config is properly configured
- Remote platforms can authenticate with your keys

**Interactive mode:**
```bash
git-keys keychain add
# Add git-keys-github-myusername-ed25519 to Keychain? [Y/n]: <Enter>
#   ✓ Added
# Add git-keys-gitlab-workuser-ed25519 to Keychain? (already in agent) [Y/n]: n
#   ⊘ Skipped
```

#### `git-keys keychain remove`

Remove SSH keys from the SSH agent.

```bash
# Remove all keys without prompts
git-keys keychain remove --all

# Interactively remove keys (default: yes)
git-keys keychain remove
```

This will:
- Remove keys from the running SSH agent
- Keep keys in macOS Keychain (they'll reload on next SSH connection)
- Only show keys currently loaded in the agent
- Prompt for confirmation for each key (unless `--all` is used)

**Note:** Keys remain in Keychain and will be automatically re-loaded when you use SSH. To permanently remove keys from Keychain, use macOS security tools.

**Common use cases:**
```bash
# Load all keys after reboot
git-keys keychain add --all

# Clear agent before switching contexts
git-keys keychain remove --all

# Selectively manage which keys are loaded
git-keys keychain add  # Interactive
```

### Health & Validation

#### `git-keys status`

Show health and status of managed SSH keys.

```bash
# Overview status
git-keys status

# Detailed status with all personas/platforms
git-keys status --verbose
```

Displays:
- Configuration file status
- Persona and platform overview
- Key status breakdown (active, revoked, expired)
- Health checks:
  - Missing key files
  - Key age monitoring (warns for keys >90 days old)
  - Expired keys
- Recommendations for fixing issues

#### `git-keys validate`

Validate git-keys configuration and setup.

```bash
# Validate configuration
git-keys validate

# Validate and automatically fix issues
git-keys validate --fix
```

Checks:
- YAML syntax validity
- Duplicate persona/platform detection
- Valid platform types
- Missing required fields
- Email format correctness
- Key file existence
- Key file permissions (should be 600)
- Fingerprint verification
- Key status validity

With `--fix`, automatically corrects:
- Insecure file permissions

### Key Lifecycle Management

#### `git-keys rotate`

Rotate SSH keys (atomic 7-step process).

```bash
# Rotate keys for specific persona
git-keys rotate personal

# Rotate all keys
git-keys rotate --all

# Preview rotation without executing
git-keys rotate --dry-run

# Rotate keys for specific platform
git-keys rotate --persona personal --platform github
```

Atomic rotation process:
1. Generate new key pair
2. Upload new key to platform
3. Verify new key works
4. Update SSH config
5. Remove old key from platform
6. Archive old key locally
7. Update configuration

#### `git-keys revoke`

Revoke SSH keys from remote platforms.

```bash
# Revoke all keys for a persona
git-keys revoke personal

# Revoke all keys
git-keys revoke --all

# Also delete local key files
git-keys revoke --all --local

# Revoke specific key by fingerprint
git-keys revoke --fingerprint SHA256:abc123...

# Revoke for specific platform
git-keys revoke --persona personal --platform github
```

Options:
- `--all`: Revoke all keys
- `--local`: Also delete local key files
- `--fingerprint <hash>`: Revoke specific key
- `--persona <name>`: Revoke keys for specific persona
- `--platform <type>`: Revoke keys for specific platform

### Backup & Recovery

#### `git-keys rebuild`

Intelligent rebuild with backup and guided re-setup.

```bash
# Interactive rebuild
git-keys rebuild --interactive

# Preview what would be cleaned up
git-keys rebuild --dry-run

# Keep remote keys, only clean locally
git-keys rebuild --interactive --keep-remote

# Quick cleanup only (no interactive re-setup)
git-keys rebuild
```

**Flags:**
- `--interactive` / `-i`: Interactive guided setup after cleanup
- `--dry-run`: Show what would be cleaned without making changes
- `--keep-remote`: Don't revoke keys from remote platforms
- `--skip-backup`: Skip creating backup (not recommended)

**Interactive Mode Features:**
- Platform detection from your git repos
- One account per platform (clean configuration)
- Rename personas (e.g., "Kun Lu" → "work")
- Manual platform override
- Group/namespace awareness

**Use Cases:**
- Starting fresh while preserving knowledge of old setup
- Migrating from manual SSH key management
- Recovering from misconfiguration
- Testing different configurations safely
- Filtering out cloned 3rd-party repos

#### `git-keys restore`

Restore configuration from a backup.

```bash
# List available backups
git-keys restore

# Restore from specific backup
git-keys restore backup-2024-01-15-143022.json

# Force restore without confirmation
git-keys restore backup.json --force
```

Restores:
- git-keys configuration file (`~/.git-keys.yaml`)
- Overview of what was backed up

Does NOT restore (must be regenerated):
- SSH keys (regenerate with `git-keys apply`)
- SSH config blocks (recreated by `git-keys apply`)
- Remote keys (recreated by `git-keys apply`)

## Configuration File

### API Tokens Setup

For automatic key upload to GitHub/GitLab, create a `.env` file in the git-keys project directory:

```bash
# .env
GITHUB_API_TOKEN_myusername=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
GITLAB_TOKEN_workuser=glpat-xxxxxxxxxxxxxxxxxxxxx
```

**Token Format:**
- GitHub: `GITHUB_API_TOKEN_[account]` where `[account]` matches your GitHub account name
- GitLab: `GITLAB_TOKEN_[account]` where `[account]` matches your GitLab account name

**Getting Tokens:**

**GitHub:**
1. Go to https://github.com/settings/tokens
2. Click "Generate new token (classic)"
3. Name: "git-keys"
4. Scopes: Select `admin:public_key` (or `write:public_key`)
5. Generate and copy token

**GitLab:**
1. Go to https://gitlab.com/-/profile/personal_access_tokens (or your self-hosted URL)
2. Create a personal access token
3. Name: "git-keys"
4. Scopes: Select `api`
5. Create and copy token

**Note:** The `.env` file is already in `.gitignore` to prevent accidental token commits.

### Configuration Structure

```yaml
version: "1.0"                    # Config file version

machine:                          # Machine identity
  id: "<UUID>"                    # Hardware UUID (auto-detected)
  name: "<name>"                  # Human-readable name
  os: "macOS"                     # Operating system
  os_version: "26.2"             # OS version

personas:                         # List of personas
  - name: "personal"              # Persona identifier
    email: "user@example.com"     # Git commit email
    platforms:                    # Git platforms for this persona
      - type: "github"            # github or gitlab
        account: "username"       # Account/username
        gitdir: "~/Projects/username/"  # Directory pattern for git identity
      - type: "gitlab"
        account: "workuser"
        base_url: "https://gitlab.company.com"  # For self-hosted
        gitdir: "~/Projects/work/"     # Directory pattern for git identity

defaults:                         # Default settings
  key_type: "ed25519"            # ed25519 or rsa
  ssh_config_path: "~/.ssh/config"
```

### Example Configuration

```yaml
version: "1.0"
machine:
  id: 89D9F984-AA37-53A5-B2E4-E56C17C7AC56
  name: My MacBook Pro
  os: macOS
  os_version: "26.2"
personas:
  - name: personal
    email: personal@example.com
    platforms:
      - type: github
        account: myusername
        gitdir: ~/Projects/myusername/
  - name: work
    email: work@company.com
    platforms:
      - type: gitlab
        account: workuser
        base_url: https://gitlab.company.com
        gitdir: ~/Projects/work/
defaults:
  key_type: ed25519
  ssh_config_path: /Users/username/.ssh/config
```

## SSH Config Management

`git-keys` updates your SSH config with managed blocks:

```
# BEGIN git-keys managed block - personal-github-myusername
Host github.com.personal
  HostName github.com
  User git
  IdentityFile ~/.ssh/git-keys-github-myusername-ed25519
  IdentitiesOnly yes
# END git-keys managed block
```

Use persona-specific hosts when cloning:

```bash
# Clone with personal account
git clone git@github.com.personal:username/repo.git

# Clone with work account  
git clone git@github.com.work:company/repo.git
```

## Backups

### Automatic Backups

`git-keys` creates backups during critical operations:

#### Rebuild Backups

When running `git-keys rebuild`:

1. **Full state backup**: `~/.git-keys/backups/backup-YYYY-MM-DD-HHMMSS.json`
   - Old configuration
   - Scan results of all SSH keys
   - Recommended persona/platform mappings
   - Git identity configuration

2. **SSH config backup**: `~/.ssh/config.pre-rebuild-YYYY-MM-DD-HHMMSS`
   - Complete copy of SSH config before cleanup

3. **Config file backup**: `~/.git-keys.yaml.pre-rebuild-YYYY-MM-DD-HHMMSS`
   - git-keys configuration before rebuild

#### Other Backups

- `git-keys apply` creates `~/.ssh/config.backup` before modifying SSH config
- Key rotation archives old keys with `.old-YYYY-MM-DD` suffix

### Restoring Backups

```bash
# List available backups
git-keys restore

# Restore specific backup
git-keys restore backup-2024-01-15-143022.json

# Then regenerate keys
git-keys apply
```

## Security Best Practices

- ✅ SSH keys generated with secure permissions (600)
- ✅ API tokens stored in macOS Keychain (never in files)
- ✅ Machine-specific keys tied to hardware UUID
- ✅ Key expiration tracking and rotation reminders
- ✅ Separate keys per persona/platform
- ✅ Fingerprint verification
- ✅ ed25519 keys by default (stronger than RSA)

## Common Workflows

### Setting Up Git Identity Switching

Git identity switching is configured per platform (since each platform account has its own SSH key).

```bash
# During 'git-keys apply', you'll be prompted for directory patterns
git-keys apply
# Prompts will ask for directory pattern for each platform:
#   personal <personal@example.com> - github/myusername: ~/Projects/myusername/
#   work <work@company.com> - gitlab/workaccount: ~/Projects/work/

# Or pre-configure in ~/.git-keys.yaml:
# personas:
#   - name: personal
#     email: personal@example.com
#     platforms:
#       - type: github
#         account: myusername
#         gitdir: ~/Projects/myusername/

# Test it works
cd ~/Projects/myusername/
git config user.email  # Shows personal email

cd ~/Projects/work/
git config user.email  # Shows work email

# To reconfigure directory patterns:
git-keys setup-git
```

### Adding a New Platform

1. Edit `~/.git-keys.yaml` to add platform
2. Preview: `git-keys plan`
3. Apply: `git-keys apply`
4. Upload public key to platform

### Rotating Old Keys

```bash
# Check key age
git-keys status --verbose

# Rotate keys >90 days old
git-keys rotate --all
```

### Recovering from Misconfiguration

```bash
# Rebuild with interactive setup
git-keys rebuild --interactive --dry-run  # Preview first
git-keys rebuild --interactive            # Then execute
```

### Moving to a New Machine

1. On old machine: Copy `~/.git-keys.yaml` to new machine
2. On new machine: Edit machine ID in config
3. Run: `git-keys apply`
4. Upload new public keys to platforms
5. Add keys to Keychain: `git-keys keychain add --all`

### Managing SSH Keys After Reboot

After restarting your Mac, SSH keys need to be loaded:

```bash
# Quick: Load all keys from Keychain
git-keys keychain add --all

# Or: Keys auto-load on first SSH use (thanks to AddKeysToAgent in SSH config)
# Just use git normally, keys load automatically
git pull  # Keys load transparently on first use

# Verify loaded keys
ssh-add -l
```

### Checking Setup Health

```bash
# Quick health check
git-keys status

# Detailed check with all info
git-keys status --verbose

# Validate configuration
git-keys validate
```

## Troubleshooting

### Configuration File Errors

```bash
# Validate your configuration
git-keys validate

# Auto-fix common issues
git-keys validate --fix
```

### Keys Not Working

```bash
# Check status
git-keys status --verbose

# Verify SSH config
cat ~/.ssh/config | grep "git-keys managed"

# Regenerate keys
git-keys apply
```

### Permission Denied

```bash
# Fix key permissions
git-keys validate --fix

# Or manually
chmod 600 ~/.ssh/git-keys-*
```

### Lost Configuration

```bash
# List backups
git-keys restore

# Restore from backup
git-keys restore backup-YYYY-MM-DD-HHMMSS.json
```

## Architecture

```
git-keys/
├── cmd/git-keys/         # Main entry point
├── internal/
│   ├── api/              # GitHub/GitLab API clients
│   ├── commands/         # CLI commands
│   │   ├── apply.go      # Apply configuration
│   │   ├── import.go     # Import existing keys
│   │   ├── init.go       # Initialize config
│   │   ├── plan.go       # Preview changes
│   │   ├── rebuild.go    # Rebuild with backup
│   │   ├── restore.go    # Restore from backup
│   │   ├── revoke.go     # Revoke keys
│   │   ├── rotate.go     # Rotate keys
│   │   ├── scan.go       # Scan existing setup
│   │   ├── status.go     # Health status
│   │   └── validate.go   # Validate config
│   ├── config/           # Configuration management
│   ├── logger/           # Logging system
│   ├── platform/         # Platform abstraction (macOS)
│   ├── sshconfig/        # SSH config management
│   └── sshkey/           # SSH key operations
└── README.md
```

## Documentation

For detailed information on how git-keys manages your configuration:

- **[Git Configuration Setup](docs/GIT_CONFIG_SETUP.md)**: Understanding platform-specific git config files, conditional includes, URL rewriting, and identity switching
- **[SSH Configuration Setup](docs/SSH_CONFIG_SETUP.md)**: How SSH config is managed, hostname sanitization, troubleshooting, and testing connections

These guides provide in-depth explanations with examples and troubleshooting tips.

## Development

### Building

```bash
go build -o git-keys ./cmd/git-keys
```

### Testing

```bash
go test ./...
```

### Running Locally

```bash
go run ./cmd/git-keys <command>
```

## Requirements

- **OS**: macOS (Linux/Windows support planned)
- **Go**: 1.21+ (for building from source)
- **Tools**: `ssh-keygen`, `ssh-add`, `git`
- **Platforms**: GitHub and/or GitLab account

## Command-Line Flags

### Global Flags

Available for all commands:

- `--config <path>`: Use custom config file (default: `~/.git-keys.yaml`)
- `--log-level <level>`: Set logging level (`error`, `warn`, `info`, `debug`, `trace`)
- `-h, --help`: Show help for any command

### Command-Specific Flags

See `git-keys <command> --help` for detailed flag information.

## Roadmap

- [x] Core key generation and management
- [x] GitHub & GitLab support
- [x] SSH config management
- [x] macOS platform support
- [x] Key scanning and import from existing setup
- [x] Automatic key rotation (7-step atomic process)
- [x] Key revocation (remote + optional local deletion)
- [x] Configuration validation
- [x] Health status monitoring
- [x] Backup and restore functionality
- [x] Dry-run mode for rebuild
- [x] Automatic key upload to platforms
- [x] Git identity management with conditional includes
- [ ] Linux/Windows platform support
- [ ] Comprehensive unit and integration tests
- [ ] Shell completion (bash, zsh, fish)
- [ ] GitHub Actions / CI pipeline
- [ ] Web dashboard for key management
- [ ] Config migration tools
- [ ] Automatic key rotation scheduling

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

## License

See [LICENSE](LICENSE) file for details.

## Support

For issues, questions, or feature requests, please open an issue on the project repository.
