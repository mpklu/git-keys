# SSH Configuration Setup by `git-keys`

## What Gets Created

### Managed Blocks in ~/.ssh/config

git-keys manages SSH configuration by adding dedicated blocks for each persona-platform combination. Each block is wrapped in clear markers to allow safe updates.

Example `~/.ssh/config`:

```ssh-config
Host *
  AddKeysToAgent yes
  UseKeychain yes


# BEGIN git-keys managed block - gelileo-github-gelileo
Host github.com.gelileo
  HostName github.com
  User git
  IdentityFile ~/.ssh/git-keys-github-fll4klu@gmail.com-ed25519
  IdentitiesOnly yes
# END git-keys managed block


# BEGIN git-keys managed block - Kun Lu-gitlab-kal
Host gitlab.domain.net.KunLu
  HostName gitlab.domain.net
  User git
  IdentityFile ~/.ssh/git-keys-gitlab-kal-ed25519
  IdentitiesOnly yes
# END git-keys managed block


# BEGIN git-keys managed block - Kun Lu-github-mpklu
Host github.com.KunLu
  HostName github.com
  User git
  IdentityFile ~/.ssh/git-keys-github-mpklu-ed25519
  IdentitiesOnly yes
# END git-keys managed block
```

## Key Features

### Platform-Specific SSH Hosts

Each persona-platform combination gets a unique SSH host alias:
- `github.com.gelileo` → Routes to GitHub using gelileo's key
- `github.com.KunLu` → Routes to GitHub using mpklu's key
- `gitlab.domain.net.KunLu` → Routes to GitLab using kal's key

### Hostname Sanitization

Persona names with spaces or special characters are automatically sanitized:
- Input: `"Kun Lu"` 
- Output: `"KunLu"` (spaces removed)
- Result: Valid hostname `github.com.KunLu` instead of invalid `github.com.Kun Lu`

This prevents SSH errors like:
```
ssh: Could not resolve hostname github.com.kun lu: nodename nor servname provided
```

### IdentitiesOnly

Each host entry uses `IdentitiesOnly yes` to ensure:
- SSH only tries the specified key for that host
- No "Too many authentication failures" errors
- Correct key is always used for each platform

### Managed Block IDs

Each block has a unique identifier based on the persona-platform combination:
- Format: `{persona}-{platform}-{account}`
- Examples: `gelileo-github-gelileo`, `Kun Lu-gitlab-kal`
- Allows git-keys to safely update specific entries without affecting others

## How It Works

### SSH Host Matching

When you run a git command, SSH matches the host:

```bash
# In directory configured for gelileo
git clone git@github.com:user/repo.git
# Git rewrites to: git@github.com.gelileo:user/repo.git
# SSH matches: Host github.com.gelileo
# Uses key: ~/.ssh/git-keys-github-fll4klu@gmail.com-ed25519

# In directory configured for mpklu  
git clone git@github.com:user/repo.git
# Git rewrites to: git@github.com.KunLu:user/repo.git
# SSH matches: Host github.com.KunLu
# Uses key: ~/.ssh/git-keys-github-mpklu-ed25519

# In directory configured for kal
git clone git@gitlab.domain.net:team/project.git
# Git rewrites to: git@gitlab.domain.net.KunLu:team/project.git
# SSH matches: Host gitlab.domain.net.KunLu
# Uses key: ~/.ssh/git-keys-gitlab-kal-ed25519
```

### Integration with Git Config

SSH configuration works hand-in-hand with git configuration:

1. **Git Config** (directory-based): Determines which identity to use based on working directory
2. **URL Rewrite**: Git config rewrites URLs to use persona-specific SSH hosts
3. **SSH Config**: SSH config routes to the correct platform with the correct key
4. **Authentication**: The correct SSH key authenticates with the platform

### Visual Flow

```
Working Directory
       ↓
Git includeIf (based on gitdir)
       ↓
Load ~/.gitconfig-{persona}-{platform}-{account}
       ↓
URL Rewrite: git@github.com: → git@github.com.{sanitized-persona}:
       ↓
SSH Config Match: Host github.com.{sanitized-persona}
       ↓
Use Specific IdentityFile
       ↓
Authenticate with Platform
```

## Testing SSH Configuration

### Test Individual Connections

Test each configured SSH host:

```bash
# Test gelileo GitHub connection
ssh -T git@github.com.gelileo
# Expected: Hi gelileo! You've successfully authenticated, but GitHub does not provide shell access.

# Test mpklu GitHub connection
ssh -T git@github.com.KunLu
# Expected: Hi mpklu! You've successfully authenticated, but GitHub does not provide shell access.

# Test kal GitLab connection
ssh -T git@gitlab.domain.net.KunLu
# Expected: Welcome to GitLab, @kal!
```

### Automated Testing

Use the `git-keys keychain add` command to automatically test all connections:

```bash
git-keys keychain add --all

# After adding keys, responds 'y' or Enter to test prompt:
Test SSH connections to verify setup? [Y/n]: 

Testing SSH connections...
  ✓ gelileo (github): Hi gelileo!
  ✓ kal (gitlab): Welcome to GitLab, @kal!
  ✓ mpklu (github): Hi mpklu!

✅ All 3 connection(s) successful!
```

### Debug SSH Issues

Enable verbose SSH output to troubleshoot:

```bash
# Maximum verbosity
ssh -vvv -T git@github.com.gelileo

# Check which identity file is being offered
ssh -v -T git@github.com.gelileo 2>&1 | grep "identity file"

# Verify key is loaded in agent
ssh-add -l
```

## Configuration Updates

### Automatic Updates

When you run `git-keys apply`:
1. Reads existing `~/.ssh/config`
2. Locates managed blocks by their markers
3. Updates or creates blocks as needed
4. Preserves all non-managed configuration

### Manual Updates

Your other SSH config remains intact:

```ssh-config
# Your custom SSH config (NOT MANAGED by git-keys)
Host myserver
  HostName example.com
  User admin
  Port 2222

Host *
  AddKeysToAgent yes
  UseKeychain yes

# git-keys managed blocks (MANAGED by git-keys)
# BEGIN git-keys managed block - ...
...
# END git-keys managed block
```

Only content between `BEGIN` and `END` markers is modified by git-keys.

## Common Patterns

### Multiple GitHub Accounts

```ssh-config
# BEGIN git-keys managed block - gelileo-github-gelileo
Host github.com.gelileo
  HostName github.com
  User git
  IdentityFile ~/.ssh/git-keys-github-gelileo-ed25519
  IdentitiesOnly yes
# END git-keys managed block

# BEGIN git-keys managed block - Kun Lu-github-mpklu
Host github.com.KunLu
  HostName github.com
  User git
  IdentityFile ~/.ssh/git-keys-github-mpklu-ed25519
  IdentitiesOnly yes
# END git-keys managed block
```

### Self-Hosted GitLab

```ssh-config
# BEGIN git-keys managed block - Kun Lu-gitlab-kal
Host gitlab.domain.net.KunLu
  HostName gitlab.domain.net
  User git
  IdentityFile ~/.ssh/git-keys-gitlab-kal-ed25519
  IdentitiesOnly yes
# END git-keys managed block
```

### Same Persona, Different Platforms

```ssh-config
# Same persona (Kun Lu) on different platforms
# BEGIN git-keys managed block - Kun Lu-github-mpklu
Host github.com.KunLu
  HostName github.com
  User git
  IdentityFile ~/.ssh/git-keys-github-mpklu-ed25519
  IdentitiesOnly yes
# END git-keys managed block

# BEGIN git-keys managed block - Kun Lu-gitlab-kal
Host gitlab.domain.net.KunLu
  HostName gitlab.domain.net
  User git
  IdentityFile ~/.ssh/git-keys-gitlab-kal-ed25519
  IdentitiesOnly yes
# END git-keys managed block
```

## Best Practices

### Host * Configuration

Keep global SSH settings in `Host *` at the top of your config:

```ssh-config
Host *
  AddKeysToAgent yes
  UseKeychain yes
  # DO NOT add IdentityFile here - it will override specific host configs
```

**Important**: Never add `IdentityFile` entries in the `Host *` section. This will cause SSH to try those keys for all connections, leading to authentication errors or using the wrong identity.

### Key Management

Use git-keys keychain commands for easy management:

```bash
# Add all keys to Keychain and test connections
git-keys keychain add --all

# Remove keys from agent
git-keys keychain remove --all

# Verify keys in agent
ssh-add -l
```

### Regular Testing

After system reboots or configuration changes:

```bash
# Quick test all connections
git-keys keychain add --all
# Respond 'y' to test prompt

# Or manually test each
ssh -T git@github.com.gelileo
ssh -T git@github.com.KunLu
ssh -T git@gitlab.domain.net.KunLu
```

## Troubleshooting

### Wrong Key Being Used

**Symptom**: SSH authenticates with wrong account

**Solution**: Check for `IdentityFile` in `Host *` section:
```bash
grep -A5 "Host \*" ~/.ssh/config | grep IdentityFile
```

If found, remove them. The `Host *` section should not contain `IdentityFile` entries.

**Verify**: Run `git-keys apply` to regenerate clean configuration.

### Hostname Resolution Errors

**Symptom**: `Could not resolve hostname github.com.kun lu`

**Cause**: Persona name has spaces, creating invalid hostname

**Solution**: git-keys automatically sanitizes hostnames. Run `git-keys apply` to fix:
- Old: `github.com.Kun Lu` (invalid)
- New: `github.com.KunLu` (valid)

### Too Many Authentication Failures

**Symptom**: `Received disconnect from X.X.X.X: Too many authentication failures`

**Cause**: SSH agent has too many keys loaded, trying all of them

**Solutions**:
1. Ensure `IdentitiesOnly yes` is set in each host block (git-keys does this automatically)
2. Remove extra keys from agent: `git-keys keychain remove --all`
3. Add back only needed keys: `git-keys keychain add --all`

### Key Not Found

**Symptom**: `Permission denied (publickey)`

**Solutions**:
1. Verify key exists: `ls -la ~/.ssh/git-keys-*`
2. Check key is in agent: `ssh-add -l`
3. Add keys to agent: `git-keys keychain add --all`
4. Test connection: `ssh -T git@github.com.{sanitized-persona}`

## Security Considerations

### IdentitiesOnly

Always use `IdentitiesOnly yes` (git-keys does this automatically):
- Prevents SSH from trying keys beyond the specified one
- Reduces exposure of SSH keys to untrusted servers
- Prevents authentication failures from trying wrong keys

### Key Isolation

Each persona-platform combination uses a dedicated key:
- Compromise of one key doesn't affect others
- Easy to revoke specific keys
- Clear audit trail of which key is used where

### Keychain Integration

On macOS, keys are stored in Keychain:
- Keys are encrypted at rest
- Automatically loaded on SSH connection
- Protected by system security

## Related Documentation

- [GIT_CONFIG_SETUP.md](GIT_CONFIG_SETUP.md) - How git-keys manages git configuration
- [README.md](../README.md) - Complete git-keys documentation
