#!/usr/bin/env bash
#
# Uninstall Fuse from GitHub Releases install.
#
#   curl -fsSL https://raw.githubusercontent.com/provasign/fuse/main/uninstall.sh | bash
#
# Environment variables (all optional):
#   INSTALL_DIR    directory where fuse was installed   (default: $HOME/bin)
#
set -euo pipefail

PRODUCT="fuse"
INSTALL_DIR="${INSTALL_DIR:-$HOME/bin}"
FUSE="${INSTALL_DIR}/${PRODUCT}"

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m✅\033[0m %s\n' "$*"; }

# Deregister fuse as the global git merge driver
if [ -x "$FUSE" ]; then
  info "Removing fuse git merge driver from global git config…"
  "$FUSE" uninstall 2>/dev/null && ok "fuse unregistered from git merge drivers" || true
fi

# Remove binary
if [ -f "$FUSE" ]; then
  rm -f "$FUSE"
  ok "removed $FUSE"
else
  info "$FUSE: not found (already removed?)"
fi

printf '\n%s uninstalled from %s\n' "$PRODUCT" "$INSTALL_DIR"
printf 'Note: .gitattributes files in individual repos may still reference the fuse merge driver.\n'
printf 'Remove the merge=fuse lines manually or run: git config --global --unset merge.fuse.driver\n'
