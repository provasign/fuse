# Fuse — AI Agent Setup Prompt

*This file is a prompt for your AI coding agent. Point any agent at it and it will install and configure Fuse for you, always at the latest version.*

---

## How to Use This File

**One command (no agent required):**

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/provasign/fuse/main/install.sh | bash

# Windows (PowerShell)
irm https://raw.githubusercontent.com/provasign/fuse/main/install.ps1 | iex
```

**Claude Code:**
```
/read AGENT_SETUP_PROMPT.md  — then say "follow the setup instructions"
```
Or from outside the repo:
```
claude "Follow the setup instructions at https://raw.githubusercontent.com/provasign/fuse/main/AGENT_SETUP_PROMPT.md"
```

**Any agent with web fetch:**
Paste this URL into chat and say "follow the setup instructions in this file":
```
https://raw.githubusercontent.com/provasign/fuse/main/AGENT_SETUP_PROMPT.md
```

---

## Instructions for the Agent

You are setting up **Fuse** — a semantic Git merge driver that resolves conflicts at the symbol level rather than the line level. Fuse uses Grove (embedded as a library) for cross-file blast radius and breaking change detection. No separate Grove installation is required.

Fuse is **not an MCP server** — it integrates as a Git merge driver, not as an AI tool plugin. There is no MCP configuration or AI tool restart required after setup.

Before Step 1, ask:

> Do you want to **install/update** Fuse, or **fully uninstall/reset** it?

- If **install/update**: continue with Step 1.
- If **uninstall/reset**: skip to **Step U1** below.

---

### Step 0 — Refresh These Instructions

Fetch the latest version of this file before doing anything else:

```
https://raw.githubusercontent.com/provasign/fuse/main/AGENT_SETUP_PROMPT.md
```

If the fetched content differs from what you are reading now, use the fetched version for all remaining steps.

---

### Step 1 — Detect the Platform

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac
echo "Platform: ${OS}-${ARCH}"
```

**Windows (PowerShell):**
```powershell
$OS = "windows"
$ARCH = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
Write-Host "Platform: $OS-$ARCH"
```

---

### Step 2 — Check for an Existing Installation

```bash
if command -v fuse &>/dev/null; then
  echo "fuse: INSTALLED at $(which fuse) — $(fuse version 2>/dev/null | head -1)"
else
  echo "fuse: not found"
fi
```

Fetch the latest release tag:

```bash
FUSE_VERSION=$(curl -sf "https://api.github.com/repos/provasign/fuse/releases/latest" \
  | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
echo "Latest: $FUSE_VERSION"
```

- If installed and up to date: tell the user and skip to Step 4.
- If older: ask whether to upgrade.
- If not found: proceed with Step 3.

---

### Step 3 — Install

**Path A — `~/bin` (no sudo; agent runs directly):**

```bash
curl -fsSL \
  "https://github.com/provasign/fuse/releases/download/${FUSE_VERSION}/install.sh" \
  | INSTALL_DIR=~/bin bash
```

**Path B — `/usr/local/bin` or any sudo-required path:**

```bash
curl -fsSL \
  "https://github.com/provasign/fuse/releases/download/${FUSE_VERSION}/install.sh" \
  -o /tmp/install-fuse.sh
```

Tell the user:
> *"Script is ready. Run this in your terminal, then come back."*
```bash
sudo INSTALL_DIR=/usr/local/bin bash /tmp/install-fuse.sh
```
Wait for the user to confirm before continuing.

**Windows (PowerShell):**
```powershell
$INSTALL_DIR = "$env:USERPROFILE\bin"   # or user-specified path
$tmpScript = "$env:TEMP\install-fuse.ps1"
Invoke-WebRequest `
  "https://github.com/provasign/fuse/releases/download/$FUSE_VERSION/install.ps1" `
  -OutFile $tmpScript
& $tmpScript -InstallDir $INSTALL_DIR
```

---

### Step 4 — Initialize

Fuse has two initialization steps: a **global** step (once per machine) and a **per-repo** step.

Ask the user for the path to their project. Then detect the git repo structure:

```bash
PROJECT="/path/to/your/project"

if git -C "$PROJECT" rev-parse --git-dir &>/dev/null 2>&1; then
  GIT_REPOS=("$PROJECT")
else
  echo "No .git at root — scanning for child repos…"
  GIT_REPOS=()
  while IFS= read -r gitdir; do
    GIT_REPOS+=("$(dirname "$gitdir")")
  done < <(find "$PROJECT" -maxdepth 2 -name ".git" -type d 2>/dev/null | sort)
  echo "Found ${#GIT_REPOS[@]} repo(s): ${GIT_REPOS[*]}"
fi
```

**Global driver registration (once per machine):**

```bash
fuse install   # registers 'fuse' merge driver in ~/.gitconfig
echo "✅ Fuse: merge driver registered globally"
```

**Per-repo `.gitattributes` (ask the user which languages they use):**

Ask:
> Which languages does this project use? I'll add Fuse `.gitattributes` entries for those only.
> Options: Go, TypeScript/JavaScript, Python, Java, Rust, C#

Then add entries only for the confirmed languages:

```bash
for REPO_DIR in "${GIT_REPOS[@]}"; do
  for EXT in go ts tsx py java rs cs; do   # only languages the user confirmed
    line="*.${EXT} merge=fuse"
    grep -qF "$line" "${REPO_DIR}/.gitattributes" 2>/dev/null \
      || echo "$line" >> "${REPO_DIR}/.gitattributes"
  done
  git -C "$REPO_DIR" add .gitattributes
  echo "✅ Fuse: .gitattributes updated in ${REPO_DIR}"
done
```

---

### Step 5 — Smoke Test

```bash
fuse version && echo "✅ fuse binary ok" || echo "❌ fuse binary failed"

echo "--- Merge driver registration ---"
{ git config merge.fuse.driver 2>/dev/null || git config --global merge.fuse.driver 2>/dev/null; } \
  && echo "✅ Fuse merge driver registered" \
  || echo "❌ Fuse not registered — run: fuse install"

echo "--- .gitattributes ---"
for REPO_DIR in "${GIT_REPOS[@]}"; do
  if [ -f "${REPO_DIR}/.gitattributes" ] && grep -q "merge=fuse" "${REPO_DIR}/.gitattributes"; then
    echo "✅ .gitattributes ok: ${REPO_DIR}"
  else
    echo "❌ .gitattributes missing fuse entries: ${REPO_DIR}"
  fi
done
```

**Common failures:**

| Symptom | Fix |
|---------|-----|
| `command not found` | Install directory not on `$PATH` — add it and restart shell |
| macOS "cannot be opened because the developer cannot be verified" | `xattr -d com.apple.quarantine $(which fuse)` |
| macOS `zsh: killed` (exit 137) | `codesign -f -s - $(which fuse)` |
| `git config merge.fuse.driver` returns nothing | Run `fuse install` to register the driver globally |
| Merge uses standard driver despite `.gitattributes` | Confirm the `fuse` binary is on `$PATH` during the merge (`which fuse`) |

---

### Step 6 — Report to the User

```
Fuse installation complete
══════════════════════════════════════════
 fuse  vX.Y.Z  ✅  ~/bin/fuse
══════════════════════════════════════════

Next steps
──────────
  Your next `git merge` in each configured repo uses symbol-aware resolution automatically.
  Conflict audit log: .git/fuse/conflict-<hash>.md  (written on unresolved conflicts)
  Breaking change report: .git/fuse/breaking-<hash>.md

Documentation: https://github.com/provasign/fuse
```

---

## Step U1 — Uninstall / Reset

Ask for the target project path.

```bash
PROJECT="/path/to/your/project"
INSTALL_DIR="${INSTALL_DIR:-$HOME/bin}"

FUSE_VERSION=$(curl -sf "https://api.github.com/repos/provasign/fuse/releases/latest" \
  | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
curl -fsSL \
  "https://github.com/provasign/fuse/releases/download/${FUSE_VERSION}/uninstall.sh" \
  | INSTALL_DIR="$INSTALL_DIR" PROJECT="$PROJECT" bash
```

**Windows (PowerShell):**
```powershell
$INSTALL_DIR = "$env:USERPROFILE\bin"
$PROJECT = "C:\path\to\project"
$FUSE_VERSION = (Invoke-RestMethod "https://api.github.com/repos/provasign/fuse/releases/latest").tag_name
$tmpScript = "$env:TEMP\uninstall-fuse.ps1"
Invoke-WebRequest `
  "https://github.com/provasign/fuse/releases/download/$FUSE_VERSION/uninstall.ps1" `
  -OutFile $tmpScript
& $tmpScript -InstallDir $INSTALL_DIR -Project $PROJECT
```

After uninstall, verify:
```bash
command -v fuse || echo "fuse removed"
git config --global merge.fuse.driver 2>/dev/null || echo "merge driver removed"
```

---

*Fuse is MIT licensed. No telemetry. Your code never leaves your machine.*
