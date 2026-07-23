# favro-mcp installer (Windows). Run in PowerShell:
#   irm https://github.com/lh-etals/favro-mcp/raw/main/install.ps1 | iex
#   or to register non-interactively afterwards:
#   favro-mcp install -email you@example.com -token <token>
$ErrorActionPreference = 'Stop'
$ProgressPreference    = 'SilentlyContinue'  # avoid PS 5.1 IWR progress slowdown

$Owner = 'lh-etals'
$Repo  = 'favro-mcp'

# --- detect arch -----------------------------------------------------------
$arch = $env:PROCESSOR_ARCHITECTURE
switch ($arch) {
  { $_ -in 'AMD64','x64' } { $target = 'windows-amd64' }
  'ARM64'                  { $target = 'windows-arm64' }
  default                  { Write-Error "Unsupported architecture: $arch"; exit 1 }
}

$Asset = "favro-mcp-$target.exe"
$Url   = "https://github.com/$Owner/$Repo/releases/latest/download/$Asset"

# --- install location ------------------------------------------------------
$InstallDir = Join-Path $env:LOCALAPPDATA 'favro-mcp'
$Target     = Join-Path $InstallDir 'favro-mcp.exe'
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

Write-Host "Downloading favro-mcp ($target)..."
try {
  Invoke-WebRequest -Uri $Url -OutFile $Target -UseBasicParsing
} catch {
  Write-Error "Download failed: $_"
  exit 1
}

# --- add to user PATH if missing ------------------------------------------
$userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if ($userPath -notlike "*$InstallDir*") {
  $newPath = if ($userPath) { "$InstallDir;$userPath" } else { $InstallDir }
  [Environment]::SetEnvironmentVariable('PATH', $newPath, 'User')
  Write-Host "Added $InstallDir to your user PATH."
  Write-Host "Restart your terminal for PATH to take effect."
} else {
  Write-Host "$InstallDir is already on your user PATH."
}

Write-Host ""
Write-Host "Installed: $Target"
Write-Host ""
Write-Host "Next: set credentials and register with your AI clients:"
Write-Host "  `$env:FAVRO_EMAIL='you@example.com'"
Write-Host "  `$env:FAVRO_API_TOKEN='<token>'"
Write-Host "  favro-mcp install"
