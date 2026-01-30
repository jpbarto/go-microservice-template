# Git Hooks

This directory contains Git hooks for the project.

## Installation

To install the hooks, run:

```bash
./hooks/install.sh
```

## Available Hooks

### pre-commit

Automatically syncs the `VERSION` file to the Helm chart's `appVersion` field whenever the VERSION file is committed.

**Usage:**
```bash
echo "1.0.1" > VERSION
git add VERSION
git commit -m "Bump version to 1.0.1"
# The hook will automatically update helm/goserv/Chart.yaml
```

## Manual Installation

If you prefer to install hooks manually:

```bash
cp hooks/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```
