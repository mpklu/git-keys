# Git Configuration Setup by `git-keys`

## What Gets Created

### 1. Platform-Specific Config Files

For each persona-platform combination, git-keys creates a dedicated config file:

**~/.gitconfig-{persona}-{platform}-{account}**

Example configurations:

**~/.gitconfig-gelileo-github-gelileo**
```gitconfig
# Git configuration for gelileo <gelileo@example.com>
# Platform: github/gelileo
# Managed by git-keys

[user]
	name = gelileo
	email = gelileo@example.com

# SSH host rewrite for platform-specific key
[url "git@github.com.gelileo:"]
	insteadOf = git@github.com:
	insteadOf = https://github.com/
```

**~/.gitconfig-Kun Lu-github-mpklu**
```gitconfig
# Git configuration for Kun Lu <mpklu@example.com>
# Platform: github/mpklu
# Managed by git-keys

[user]
	name = Kun Lu
	email = mpklu@example.com

# SSH host rewrite for platform-specific key
[url "git@github.com.KunLu:"]
	insteadOf = git@github.com:
	insteadOf = https://github.com/
```

**~/.gitconfig-Kun Lu-gitlab-kal**
```gitconfig
# Git configuration for Kun Lu <kal@company.com>
# Platform: gitlab/kal
# Managed by git-keys

[user]
	name = Kun Lu
	email = kal@company.com

# SSH host rewrite for platform-specific key
[url "git@gitlab.macpractice.net.KunLu:"]
	insteadOf = git@gitlab.macpractice.net:
	insteadOf = https://gitlab.macpractice.net/
```

### 2. Conditional Includes Added to ~/.gitconfig

```gitconfig
# Your existing ~/.gitconfig content...

# BEGIN git-keys managed conditional includes
[includeIf "gitdir:/Users/username/Projects/gelileo/"]
	path = /Users/username/.gitconfig-gelileo-github-gelileo

[includeIf "gitdir:/Users/username/Projects/mp/"]
	path = /Users/username/.gitconfig-Kun Lu-gitlab-kal

[includeIf "gitdir:/Users/username/Projects/mpklu/"]
	path = /Users/username/.gitconfig-Kun Lu-github-mpklu
# END git-keys managed conditional includes
```

## Key Features

### Platform-Level Granularity

Each persona-platform combination gets its own configuration file. This allows:
- Same persona (e.g., "Kun Lu") with different accounts on different platforms
- Different identities for different platforms even with the same persona
- Fine-grained control over which identity is used where

### Hostname Sanitization

Persona names with spaces (e.g., "Kun Lu") are automatically sanitized for use in hostnames:
- `"Kun Lu"` → `"KunLu"` (removes spaces and special characters)
- This ensures valid SSH hostnames like `github.com.KunLu` instead of invalid `github.com.Kun Lu`
- Only affects hostnames; your actual git identity (name/email) remains unchanged

### Directory-Based Identity Switching

When you work in `/Users/username/Projects/gelileo/`:
- Git uses identity: `gelileo <gelileo@example.com>`
- Clones automatically use: `git@github.com.gelileo:` (gelileo's SSH key)
- Commits are signed with gelileo identity

When you work in `/Users/username/Projects/mpklu/`:
- Git uses identity: `Kun Lu <mpklu@example.com>`
- Clones automatically use: `git@github.com.KunLu:` (mpklu's SSH key)
- Commits are signed with Kun Lu identity

When you work in `/Users/username/Projects/mp/`:
- Git uses identity: `Kun Lu <kal@company.com>`
- Clones automatically use: `git@gitlab.macpractice.net.KunLu:` (kal's SSH key on GitLab)
- Commits are signed with Kun Lu identity for work

### URL Rewriting

The `insteadOf` configuration automatically rewrites URLs:

```bash
# In ~/Projects/gelileo/
git clone git@github.com:user/repo.git
# Becomes: git@github.com.gelileo:user/repo.git

# Or with HTTPS
git clone https://github.com/user/repo.git
# Becomes: git@github.com.gelileo:user/repo.git

# In ~/Projects/mpklu/
git clone git@github.com:user/repo.git
# Becomes: git@github.com.KunLu:user/repo.git

# In ~/Projects/mp/
git clone git@gitlab.macpractice.net:team/project.git
# Becomes: git@gitlab.macpractice.net.KunLu:team/project.git
```

This ensures you always use the correct SSH key for each repository.

## How It Works

### Setup Process

1. **Configuration Prompt**: `git-keys setup-git` (or `git-keys apply`) prompts for directory patterns
2. **Platform Config Creation**: Creates `~/.gitconfig-{persona}-{platform}-{account}` files
3. **Conditional Includes**: Updates `~/.gitconfig` with `includeIf` directives
4. **Hostname Sanitization**: Automatically sanitizes persona names for SSH hostnames
5. **URL Rewrites**: Sets up automatic URL rewriting for each platform

### Testing

After running `git-keys setup-git` or `git-keys apply`:

```bash
# Test gelileo identity (github)
cd ~/Projects/gelileo/
git config user.name    # Returns: gelileo
git config user.email   # Returns: gelileo@example.com

# Test mpklu identity (github)
cd ~/Projects/mpklu/
git config user.name    # Returns: Kun Lu
git config user.email   # Returns: mpklu@example.com

# Test kal identity (gitlab)
cd ~/Projects/mp/
git config user.name    # Returns: Kun Lu
git config user.email   # Returns: kal@company.com

# Test URL rewriting
cd ~/Projects/gelileo/
git config --get-urlmatch url.insteadof git@github.com:user/repo.git
# Returns: git@github.com.gelileo:

cd ~/Projects/mpklu/
git config --get-urlmatch url.insteadof git@github.com:user/repo.git
# Returns: git@github.com.KunLu:

cd ~/Projects/mp/
git config --get-urlmatch url.insteadof git@gitlab.macpractice.net:team/project.git
# Returns: git@gitlab.macpractice.net.KunLu:
```

## Updating Configuration

Re-run `git-keys setup-git` to update the configuration:
- It will backup your existing `~/.gitconfig`
- Replace the managed section with updated entries
- Keep all your other git configuration intact

## Removing Configuration

To manually remove git-keys git configuration:

1. Remove the managed section from `~/.gitconfig`:
   ```bash
   # Remove lines between:
   # BEGIN git-keys managed conditional includes
   # END git-keys managed conditional includes
   ```

2. Remove platform config files:
   ```bash
   rm ~/.gitconfig-*-github-*
   rm ~/.gitconfig-*-gitlab-*
   ```

## Restore from Backup

If something goes wrong, restore from backup:

```bash
cp ~/.gitconfig.backup-git-keys ~/.gitconfig
```

## Related Documentation

See also: [SSH_CONFIG_SETUP.md](SSH_CONFIG_SETUP.md) for details on how git-keys manages SSH configuration.
