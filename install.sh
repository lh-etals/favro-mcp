#!/bin/sh
# favro-mcp installer (Linux / macOS). Run with:
#   curl -fsSL https://github.com/lh-etals/favro-mcp/raw/main/install.sh | sh
#   or:  curl -fsSL <url> | sh -s -- --yes   (then register non-interactively)
set -e

OWNER="lh-etals"
REPO="favro-mcp"

# --- detect OS / arch ------------------------------------------------------
OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
  Linux*)  os=linux ;;
  Darwin*) os=darwin ;;
  *) printf 'Unsupported OS: %s\n' "$OS" >&2; exit 1 ;;
esac
case "$ARCH" in
  x86_64|amd64)   arch=amd64 ;;
  arm64|aarch64)  arch=arm64 ;;
  *) printf 'Unsupported architecture: %s\n' "$ARCH" >&2; exit 1 ;;
esac

ASSET="favro-mcp-${os}-${arch}"
URL="https://github.com/${OWNER}/${REPO}/releases/latest/download/${ASSET}"

# --- install location ------------------------------------------------------
INSTALL_DIR="$HOME/.favro-mcp/bin"
TARGET="$INSTALL_DIR/favro-mcp"
mkdir -p "$INSTALL_DIR"

printf '\n  favro-mcp installer\n\n  Downloading...\n\n'
# curl --progress-bar writes the bar to stderr so it shows under `sh` but
# never pollutes the captured stdout of `curl | sh`.
if command -v curl >/dev/null 2>&1; then
  if ! curl -fSL --progress-bar "$URL" -o "$TARGET"; then
    printf '\n  Download failed. Please check your connection and try again.\n  URL: %s\n' "$URL" >&2
    rm -f "$TARGET"
    exit 1
  fi
elif command -v wget >/dev/null 2>&1; then
  if ! wget -q --show-progress -O "$TARGET" "$URL"; then
    printf '\n  Download failed. Please check your connection and try again.\n  URL: %s\n' "$URL" >&2
    rm -f "$TARGET"
    exit 1
  fi
else
  printf 'Neither curl nor wget is available; cannot download.\n' >&2
  exit 1
fi
chmod +x "$TARGET"

# --- add to PATH if missing (silent on success) ----------------------------
case ":$PATH:" in
  *":$INSTALL_DIR:"*) on_path=1 ;;
  *) on_path=0 ;;
esac

if [ "$on_path" -eq 0 ]; then
  line="export PATH=\"$INSTALL_DIR:\$PATH\""
  for rc in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.profile" "$HOME/.bash_profile"; do
    [ -f "$rc" ] || continue
    if ! grep -qF "$INSTALL_DIR" "$rc" 2>/dev/null; then
      printf '\n# added by favro-mcp installer\n%s\n' "$line" >> "$rc"
    fi
    on_path=2
    break
  done
fi

# If we have a controlling terminal, run the interactive setup right away. Under
# `curl | sh` stdin is the script pipe, so we read from /dev/tty to reach the
# user's terminal directly.
if [ -t 0 ] || [ -e /dev/tty ]; then
  if [ -f "$HOME/.favro-mcp/credentials.json" ]; then
    printf '\nFavro credentials already configured (run `favro-mcp login` to change).\n'
  fi
  printf '\n=== Configuring favro-mcp (login + toolset + clients) ===\n'
  "$TARGET" configure </dev/tty || printf '  (configure skipped or failed; run `favro-mcp configure` later)\n'
else
  printf '\nThen run:\n  favro-mcp configure\n'
fi
