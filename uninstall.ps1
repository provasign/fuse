# Uninstall Fuse from GitHub Releases install.
#
#   irm https://raw.githubusercontent.com/provasign/fuse/main/uninstall.ps1 | iex
#
# Parameters (pass as env vars or dot-source):
#   $env:INSTALL_DIR   directory where fuse was installed   (default: $HOME\bin)
#
[CmdletBinding()]
param(
  [string]$InstallDir = $env:INSTALL_DIR
)
$ErrorActionPreference = "Stop"
$PRODUCT = "fuse"
if (-not $InstallDir) { $InstallDir = "$env:USERPROFILE\bin" }

function ok($msg)   { Write-Host "✅ $msg" -ForegroundColor Green }
function info($msg) { Write-Host "==> $msg" -ForegroundColor Blue }

$fuseExe = "$InstallDir\$PRODUCT.exe"

# Deregister git merge driver
if (Test-Path $fuseExe) {
  info "Removing fuse git merge driver from global git config…"
  & $fuseExe uninstall 2>$null; ok "fuse unregistered from git merge drivers"
}

# Remove binary
if (Test-Path $fuseExe) {
  Remove-Item $fuseExe -Force
  ok "removed $fuseExe"
} else {
  info "$fuseExe : not found (already removed?)"
}

Write-Host ""
Write-Host "$PRODUCT uninstalled from $InstallDir"
