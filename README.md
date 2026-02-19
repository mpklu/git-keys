# git-keys

> Automated SSH key management for Git platforms (GitHub & GitLab)

`git-keys` is a command-line tool that simplifies SSH key management across GitHub and GitLab. It supports multiple personas (personal, work, etc.), automatic key generation, SSH config management, and follows security best practices.

## Features

- **Multi-Persona Support**: Separate SSH keys for personal, work, and other identities
- **Platform Support**: GitHub and self-hosted GitLab
- **Automated Key Generation**: Creates ed25519 or RSA keys with proper permissions
- **SSH Config Management**: Automatically updates `~/.ssh/config` with managed blocks
- **Secure Token Storage**: Uses macOS Keychain for API tokens
- **Key Expiration**: Track key age and rotation schedules
- **Machine-Specific**: Keys are tied to your machine's hardware UUID

## Installation

### From Source

```bash
cd git-keys
go build -o git-keys ./cmd/git-keys
sudo mv git-keys /usr/local/bin/
```
### Keychain (Mac OS)

- To remove the invalid GitLab token from your Keychain:

```
security delete-generic-password -s "gitlab-api-test" -a "git-keys"
```
- Verify it's removed
```
# This should fail with "password could not be found"
security find-generic-password -s "gitlab-api-test" -a "git-keys" -w
```


## Quick Start

### Migration from Existing Setup

If you already have SSH keys and configurations:

#### 1. Scan Your Current Setup

```bash
git-keys scan
```

This will discover:
- Existing SSH keys in `~/.ssh/`
- SSH config entries
- Git identity configuration
- Keys loaded in SSH agent
- Optionally, keys registered on GitHub/GitLab (use `--check-remote`)

#### 2. Import Your Keys

```bash
git-keys import
```

Interactive wizard that will:
- Map your existing keys to personas
- Optionally reorganize keys into git-keys structure
- Update SSH config with managed blocks
- Create git-keys configuration

All changes are backed up and reversible.

### Fresh Start

###1. Initialize Configuration

```bash
git-keys init
```

This will:
- Detect your machine's hardware UUID
- Create `~/.git-keys.yaml` configuration file
- Guide you through setting up your first persona

### 2. Configure Personas

Edit `~/.git-keys.yaml` to add your personas and platforms:

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
  - name: work
    email: work@company.com
    platforms:
      - type: gitlab
        account: workuser
        base_url: https://gitlab.company.com
defaults:
  key_type: ed25519
  ssh_config_path: /Users/username/.ssh/config
```

### 3. Plan Changes

```bash
git-keys plan
```

See what changes will be made before applying them.

### 4. Apply Configuration

```bash
git-keys apply
```

This will:
- Generate SSH keys for each persona/platform
- Update your SSH config with managed blocks
- Display public keys to upload to platforms

## Usage

### Commands

#### Discovery & Migration

- `git-keys scan` - Discover existing SSH keys and configuration
  - `--check-remote` - Query GitHub/GitLab for registered keys
  - `--json` - Output as JSON
  - `--path <dir>` - Scan specific directory (default: `~/.ssh`)

- `git-keys import` - Import existing SSH keys into git-keys
  - `--interactive` - Interactive wizard (default)
  - `--dry-run` - Show what would be imported
  - `--auto` - Attempt automatic mapping

#### Setup & Management

- `git-keys init` - Initialize configuration
- `git-keys plan` - Show what changes will be made
- `git-keys apply` - Generate keys and update SSH config

#### Key Lifecycle Management

- `git-keys rotate [persona]` - Rotate SSH keys (atomic 7-step process)
  - `--all` - Rotate all keys
  - `--dry-run` - Show what would be rotated
  - `--persona <name>` - Rotate keys for specific persona

- `git-keys revoke [persona]` - Revoke SSH keys from remote platforms
  - `--all` - Revoke all keys
  - `--local` - Also delete local key files
  - `--fingerprint <hash>` - Revoke specific key by fingerprint
  - `--persona <name>` - Revoke keys for specific persona
  - `--platform <type>` - Revoke keys for specific platform

- `git-keys --help` - Show help

### Command Flags

- `--config <path>` - Use custom config file (default: `~/.git-keys.yaml`)
- `--log-level <level>` - Set logging level (error, warn, info, debug, trace)
- `-y, --yes` - Skip confirmation prompts (apply command)

## Configuration File

### Structure

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
      - type: "gitlab"
        account: "workuser"
        base_url: "https://gitlab.company.com"  # For self-hosted

defaults:                         # Default settings
  key_type: "ed25519"            # ed25519 or rsa
  ssh_config_path: "~/.ssh/config"
```

## SSH Config Management

`git-keys` updates your SSH config with managed blocks that look like this:

```
# BEGIN git-keys managed block - personal-github-myusername
Host github.com.personal
  HostName github.com
  User git
  IdentityFile ~/.ssh/git-keys-github-myusername-ed25519
  IdentitiesOnly yes

# END git-keys managed block
```

This allows you to use persona-specific hosts:

```bash
# Clone with personal account
git clone git@github.com.personal:username/repo.git

# Clone with work account  
git clone git@github.com.work:company/repo.git
```

## Security

- SSH keys are generated with secure permissions (600)
- API tokens stored in macOS Keychain
- Machine-specific keys tied to hardware UUID
- Support for key expiration tracking
- Separate keys per persona/platform

## Architecture

```
git-keys/
├── cmd/git-keys/         # Main entry point
├── internal/
│   ├── api/              # GitHub/GitLab API clients
│   ├── commands/         # CLI commands
│   ├── config/           # Configuration management
│   ├── logger/           # Logging system
│   ├── platform/         # Platform abstraction (macOS)
│   ├── sshconfig/        # SSH config management
│   └── sshkey/           # SSH key operations
└── README.md
```

## Development

### Building

```bash
go build -o git-keys ./cmd/git-keys
```

### Testing

```bash
go test ./...
```

## Requirements

- macOS (current version - other platforms planned)
- Go 1.21+ (for building from source)
- SSH tools (`ssh-keygen`, etc.)
- GitHub/GitLab account

## Roadmap

- [x] Core key generation and management
- [x] GitHub & GitLab support
- [x] SSH config management
- [x] macOS platform support
- [x] Key scanning and import from existing setup
- [x] Automatic key rotation (7-step atomic process)
- [x] Key revocation (remote + optional local deletion)
- [ ] Linux/Windows support
- [ ] Unit and integration tests
- [ ] Shell completion
- [ ] GitHub Actions / CI pipeline
