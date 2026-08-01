# favro-mcp installer (Windows). Run in PowerShell:
#   irm https://github.com/lh-etals/favro-mcp/raw/main/install.ps1 | iex
$ErrorActionPreference = 'Stop'
# Invoke-WebRequest's built-in progress bar is slow and flickers badly in
# Windows PowerShell; the download below renders its own.
$ProgressPreference    = 'SilentlyContinue'

$Owner  = 'lh-etals'
$Repo   = 'favro-mcp'
$Binary = 'favro-mcp'

# --- detect arch -----------------------------------------------------------
$arch = $env:PROCESSOR_ARCHITECTURE
switch ($arch) {
  { $_ -in 'AMD64','x64' } { $target = 'windows-amd64' }
  'ARM64'                  { $target = 'windows-arm64' }
  default                  { Write-Host "  Unsupported architecture: $arch" -ForegroundColor Red; return }
}

$Asset = "$Binary-$target.exe"
$Url   = "https://github.com/$Owner/$Repo/releases/latest/download/$Asset"

# --- install location ------------------------------------------------------
$InstallDir = Join-Path $env:LOCALAPPDATA $Repo
$Target     = Join-Path $InstallDir "$Binary.exe"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

Write-Host ""
Write-Host "  favro-mcp installer" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Downloading $Asset..."

# Always download beside the target, never over it.
#
# Clearing the old binary first meant a re-install that could not download -
# offline, a 5xx from the release host, a full disk - left no binary at all,
# with the user PATH still pointing at an empty directory. The shell installer
# has always downloaded to a temporary file and moved it into place only after
# the bytes are on disk; this does the same.
$tempTarget = "$Target.new"

# Sweep up binaries displaced by earlier installs, now that whatever had them
# open has most likely exited.
Get-ChildItem -Path $InstallDir -Filter "$Binary.exe.old-*" -ErrorAction SilentlyContinue |
  ForEach-Object { Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue }

$request = $null
$response = $null
$stream = $null
$fs = $null
try {
  $request = [System.Net.HttpWebRequest]::Create($Url)
  $request.Method = "GET"
  $response = $request.GetResponse()
  $total = [int]$response.ContentLength
  $stream = $response.GetResponseStream()
  $fs = [System.IO.File]::Create($tempTarget)
  $buffer = New-Object byte[] 65536
  $downloaded = 0
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  $lastUpdate = 0

  $renderBar = {
    param($dl, $tot, $el)
    $pct = if ($tot -gt 0) { [math]::Round($dl / $tot * 100) } else { 0 }
    if ($pct -gt 100) { $pct = 100 }
    $filled = [math]::Floor($pct / 5)
    if ($filled -gt 20) { $filled = 20 }
    $bar = ('#' * $filled).PadRight(20)
    # One decimal always, so a whole number reads as 3.0 rather than 3 and the
    # line does not change width as it counts up. The shell installer renders
    # the identical line; the two should be indistinguishable.
    $dlMB = [math]::Round($dl / 1048576, 1)
    $totMB = [math]::Round($tot / 1048576, 1)
    $speed = if ($el -gt 0) { [math]::Round($dlMB / $el, 1) } else { 0 }
    $eta = if ($speed -gt 0) { [math]::Round((($totMB - $dlMB)) / $speed) } else { 0 }
    $line = ("  [{0}] {1,3}%  {2:0.0}/{3:0.0} MB  {4:0.0} MB/s  ETA {5:00}s" -f $bar, $pct, $dlMB, $totMB, $speed, $eta)
    # Pad so shorter lines fully overwrite the previous render (no stale chars).
    Write-Host ("`r{0}" -f $line.PadRight(72)) -NoNewline
  }

  while (($read = $stream.Read($buffer, 0, $buffer.Length)) -gt 0) {
    $fs.Write($buffer, 0, $read)
    $downloaded += $read
    $now = [System.Environment]::TickCount
    if ($now - $lastUpdate -gt 200) {
      $lastUpdate = $now
      & $renderBar $downloaded $total $sw.Elapsed.TotalSeconds
    }
  }
  # Force a final 100% render.
  & $renderBar $downloaded $total $sw.Elapsed.TotalSeconds
  Write-Host ""
} catch {
  Write-Host ""
  Write-Host "  Download failed. Please check your connection and try again." -ForegroundColor Red
  Write-Host "  URL: $Url" -ForegroundColor Red
  Write-Host "  Reason: $_" -ForegroundColor Red
  # Only the partial download is removed. Whatever was installed before is
  # still there and still works.
  Remove-Item $tempTarget -ErrorAction SilentlyContinue
  return
} finally {
  if ($fs -ne $null) { $fs.Close() }
  if ($stream -ne $null) { $stream.Close() }
  if ($response -ne $null) { $response.Close() }
}

# Nothing was downloaded, so nothing is replaced.
if (-not (Test-Path $tempTarget) -or (Get-Item $tempTarget).Length -eq 0) {
  Remove-Item $tempTarget -ErrorAction SilentlyContinue
  Write-Host "  Download did not complete; nothing was installed." -ForegroundColor Red
  return
}

# Swap the new binary into place by moving the old one aside first.
#
# Two Windows facts make this the only reliable order. Windows PowerShell's
# Move-Item -Force does not overwrite an existing destination, so moving onto
# the target fails with "Cannot create a file when that file already exists".
# And the file being replaced is usually running - an MCP client holds
# favro-mcp.exe open for as long as it is connected - so deleting it first fails
# too. Windows refuses to delete a running image but does allow renaming one,
# which is what this relies on.
#
# The displaced file gets a unique name, so a copy still held open by a running
# client cannot block the next install. Whatever is left is swept up above on a
# later run, once nothing has it open.
$oldTarget = "$Target.old-$([System.Guid]::NewGuid().ToString('N').Substring(0,8))"
$movedAside = $false
if (Test-Path $Target) {
  try {
    Move-Item $Target $oldTarget
    $movedAside = $true
  } catch {
    Write-Host ""
    Write-Host "  Could not replace $Binary.exe: $_" -ForegroundColor Red
    Write-Host "  Close any running favro-mcp process and re-run the installer." -ForegroundColor Yellow
    Remove-Item $tempTarget -ErrorAction SilentlyContinue
    return
  }
}
try {
  Move-Item $tempTarget $Target
} catch {
  Write-Host ""
  Write-Host "  Could not replace $Binary.exe: $_" -ForegroundColor Red
  # Put the working binary back rather than leaving nothing installed.
  if ($movedAside) { Move-Item $oldTarget $Target -ErrorAction SilentlyContinue }
  Remove-Item $tempTarget -ErrorAction SilentlyContinue
  return
}
# Fails while a client still has it open, which is expected and harmless.
if ($movedAside) { Remove-Item $oldTarget -Force -ErrorAction SilentlyContinue }

# --- add to user PATH if missing -------------------------------------------
# SetEnvironmentVariable updates the stored user PATH, which only new processes
# read. This shell keeps the PATH it started with, so the command is not found
# here no matter what we do -- the user has to be told, or the very next thing
# they type fails with "not recognized".
$pathChanged = $false
$userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if ($userPath -notlike "*$InstallDir*") {
  $newPath = if ($userPath) { "$InstallDir;$userPath" } else { $InstallDir }
  [Environment]::SetEnvironmentVariable('PATH', $newPath, 'User')
  $pathChanged = $true
}
# Make it work in this session too, so `configure` below and anything the user
# tries immediately afterwards can find the binary without reopening a console.
if ($env:PATH -notlike "*$InstallDir*") {
  $env:PATH = "$InstallDir;$env:PATH"
}

# --- launch the configurer --------------------------------------------------
try {
  & $Target configure
} catch {
  Write-Host "  configure did not complete: $_" -ForegroundColor Red
  Write-Host "  Re-run ``$Binary configure`` later to finish setup." -ForegroundColor Yellow
}

# --- tell the user what to do next ------------------------------------------
if ($pathChanged) {
  Write-Host ""
  Write-Host "  Added to your PATH: $InstallDir" -ForegroundColor Cyan
  Write-Host ""
  Write-Host "  Open a NEW PowerShell window before running $Binary." -ForegroundColor Yellow
  Write-Host "  This window still has the PATH it started with, so the command" -ForegroundColor Yellow
  Write-Host "  will not be found here." -ForegroundColor Yellow
  Write-Host ""
  Write-Host "  In a new window:  $Binary"
  Write-Host "  Or right now:     & '$Target'"
}
