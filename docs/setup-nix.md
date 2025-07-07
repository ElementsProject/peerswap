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
```bash
git clone https://github.com/YusukeShimizu/peerswap.git
cd peerswap

# With direnv (automatic)
direnv allow

# Without direnv (manual)
nix develop
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

### 3. Local Cache Push

To push from local development to the cache:

```bash
# Create a profile and push to cache
nix develop --profile dev-profile
cachix push peerswap dev-profile
```

## How It Works

### Local Development

The `.envrc` file automatically configures Cachix:

```bash
use flake

# Setup Cachix for development
if command -v cachix >/dev/null 2>&1; then
  cachix use peerswap
fi
```

## References

- [Nix Flakes](https://nixos.wiki/wiki/Flakes)
- [Cachix Documentation](https://docs.cachix.org/)
- [GitHub Actions + Nix](https://nix.dev/guides/recipes/continuous-integration-github-actions)
