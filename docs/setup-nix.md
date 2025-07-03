# Nix Development Environment & Cachix Setup

This document explains how to set up and maintain the Nix development environment for PeerSwap, including Cachix binary cache configuration.

## Overview

PeerSwap uses Nix flakes for reproducible development environments and Cachix for fast binary caching. This setup provides:
- Deterministic build environments across all platforms
- Fast CI/CD builds through binary caching
- Consistent developer experience

## Quick Start for Developers

### Prerequisites

1. **Install Nix** (if not already installed):
   ```bash
   # Multi-user installation (recommended)
   curl -L https://nixos.org/nix/install | sh -s -- --daemon
   
   # Enable flakes
   mkdir -p ~/.config/nix
   echo "experimental-features = nix-command flakes" >> ~/.config/nix/nix.conf
   ```

2. **Install direnv** (optional but recommended):
   ```bash
   # On macOS
   brew install direnv
   
   # On Linux
   curl -sfL https://direnv.net/install.sh | bash
   
   # Add to shell profile
   echo 'eval "$(direnv hook bash)"' >> ~/.bashrc  # or ~/.zshrc
   ```

### Development Environment

1. **Clone and enter the project**:
   ```bash
   git clone https://github.com/YusukeShimizu/peerswap.git
   cd peerswap
   
   # With direnv (automatic)
   direnv allow
   
   # Without direnv (manual)
   nix develop
   ```

2. **Verify setup**:
   ```bash
   make bins     # Build binaries
   make test     # Run tests
   ```

### Binary Cache (Cachix)

The project automatically uses the `peerswap` Cachix cache for faster builds. If you don't have Cachix installed:

```bash
# Install Cachix
nix profile install nixpkgs#cachix

# Use the cache (done automatically by .envrc)
cachix use peerswap
```

## Maintainer Setup Guide

### 1. Cachix Cache Creation

1. **Create Cachix account**:
   - Visit https://app.cachix.org/
   - Sign up with GitHub account
   - Verify email address

2. **Access existing cache**:
   - The cache `peerswap` already exists at https://app.cachix.org/cache/peerswap
   - Ensure you have admin access to this cache
   - If you don't have access, contact the cache owner

### 2. Authentication Token

1. **Generate token**:
   - Go to cache dashboard → `peerswap` (https://app.cachix.org/cache/peerswap)
   - Navigate to "Auth tokens" tab
   - Click "Generate token"
   - Select permissions: **Push** (required for CI)
   - Copy the generated token

2. **Add GitHub repository secret**:
   - Go to GitHub repository: Settings → Secrets and variables → Actions
   - Click "New repository secret"
   - Name: `CACHIX_AUTH_TOKEN`
   - Value: Paste the token from step 1
   - Click "Add secret"

### 3. Verification

1. **Test CI**:
   - Push changes or create a pull request
   - Check GitHub Actions logs for:
     ```
     Cachix: cache peerswap configured
     ```

2. **Monitor performance**:
   - Compare build times before/after Cachix
   - Expected improvement: 50-80% faster builds

## How It Works

### CI/CD Pipeline

The GitHub Actions workflow (`.github/workflows/ci.yml`) includes:

```yaml
- uses: cachix/install-nix-action@v31
  with:
    github_access_token: ${{ secrets.GITHUB_TOKEN }}
    nix_path: nixpkgs=channel:nixos-unstable
    extra_nix_config: |
      experimental-features = nix-command flakes

- uses: cachix/cachix-action@v16
  with:
    name: peerswap
    authToken: '${{ secrets.CACHIX_AUTH_TOKEN }}'
    extraPullNames: nix-community
    useDaemon: true
    skipPush: ${{ github.event_name == 'pull_request' && github.event.pull_request.head.repo.full_name != github.repository }}
```

### Security Model

- **Public cache**: Anyone can pull (read) from the cache
- **Authenticated push**: Only CI with valid token can push
- **Fork safety**: External forks can read but cannot push
- **Background uploads**: Non-blocking cache population

### Local Development

The `.envrc` file automatically configures Cachix:

```bash
use flake

# Setup Cachix for development
if command -v cachix >/dev/null 2>&1; then
  cachix use peerswap
fi
```

## Troubleshooting

### Common Issues

1. **"Secret not found" in CI**:
   - Verify `CACHIX_AUTH_TOKEN` is correctly named in repository secrets
   - Check token has not expired

2. **"Permission denied" when pushing**:
   - Ensure token has "Push" permission
   - Verify cache name matches exactly: `peerswap`

3. **Fork PRs cannot push**:
   - This is expected security behavior
   - Forks can read from cache but cannot write
   - No action needed

4. **Slow builds despite cache**:
   - Check if cache is being used: `nix build --show-trace`
   - Verify substituters in `~/.config/nix/nix.conf`

### Advanced Configuration

1. **Custom substituters**:
   ```bash
   # Add to ~/.config/nix/nix.conf
   substituters = https://cache.nixos.org/ https://peerswap.cachix.org
   trusted-public-keys = cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY= peerswap.cachix.org-1:...
   ```

2. **Local cache management**:
   ```bash
   # Clear local cache
   nix-collect-garbage -d
   
   # Check cache usage
   du -sh /nix/store
   ```

## Maintenance

### Regular Tasks

1. **Monitor cache usage**:
   - Check Cachix dashboard for storage usage
   - Clean up old artifacts if needed

2. **Token rotation**:
   - Rotate auth tokens periodically (recommended: every 6 months)
   - Update GitHub secrets accordingly

3. **Performance monitoring**:
   - Track CI build times
   - Monitor cache hit rates

### Updates

When updating Nix-related dependencies:

1. Update `flake.lock`:
   ```bash
   nix flake update
   ```

2. Test locally:
   ```bash
   nix develop
   make test
   ```

3. Verify CI builds successfully with new dependencies

## Support

For issues related to:
- **Nix environment**: Check [Nix documentation](https://nixos.org/manual/nix/stable/)
- **Cachix**: Visit [Cachix documentation](https://docs.cachix.org/)
- **Project-specific**: Create issue in this repository

## References

- [Nix Flakes](https://nixos.wiki/wiki/Flakes)
- [Cachix Documentation](https://docs.cachix.org/)
- [GitHub Actions + Nix](https://nix.dev/guides/recipes/continuous-integration-github-actions)