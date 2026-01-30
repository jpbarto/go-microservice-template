#!/bin/bash

# Script to install Git hooks from the hooks/ directory

HOOKS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GIT_HOOKS_DIR="$(git rev-parse --git-dir)/hooks"

echo "Installing Git hooks..."

# Copy all hook files from hooks/ to .git/hooks/
for hook in "$HOOKS_DIR"/*; do
    if [ -f "$hook" ] && [ "$(basename "$hook")" != "install.sh" ]; then
        hook_name=$(basename "$hook")
        echo "  Installing $hook_name..."
        cp "$hook" "$GIT_HOOKS_DIR/$hook_name"
        chmod +x "$GIT_HOOKS_DIR/$hook_name"
    fi
done

echo "✓ Git hooks installed successfully!"
echo ""
echo "Installed hooks:"
ls -1 "$GIT_HOOKS_DIR" | grep -v ".sample"
