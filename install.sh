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

printf 'Downloading favro-mcp (%s/%s)...\n' "$os" "$arch"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$URL" -o "$TARGET"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$TARGET" "$URL"
else
  printf 'Neither curl nor wget is available; cannot download.\n' >&2
  exit 1
fi
chmod +x "$TARGET"

# --- add to PATH if missing ------------------------------------------------
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
      printf 'Added PATH entry to %s\n' "$rc"
    fi
    on_path=2
    break
  done
fi

printf '\nInstalled: %s\n' "$TARGET"

# If we have a controlling terminal, run the interactive setup right away. Skip
# `login` if credentials already exist. Under `curl | sh` stdin is the script
# pipe, so we read from /dev/tty to reach the user's terminal directly.
if [ -t 0 ] || [ -e /dev/tty ]; then
  if [ -f "$HOME/.favro-mcp/credentials.json" ]; then
    printf '\nFavro credentials already configured (run `favro-mcp login` to change).\n'
  else
    printf '\n=== Setting up Favro credentials ===\n'
    "$TARGET" login </dev/tty 2>/dev/null || printf '  (login skipped or failed; run `favro-mcp login` later)\n'
  fi
  printf '\n=== Registering with your AI clients ===\n'
  "$TARGET" install </dev/tty 2>/dev/null || true
  if [ "$on_path" -eq 0 ]; then
    printf '\nNote: favro-mcp is at %s — reopen your shell or add it to PATH first.\n' "$INSTALL_DIR"
  fi
else
  if [ "$on_path" -eq 2 ]; then
    printf 'Restart your shell (or open a new one) so "favro-mcp" is on PATH.\n'
  elif [ "$on_path" -eq 0 ]; then
    printf 'Add it to PATH manually:\n  export PATH="%s:$PATH"\n' "$INSTALL_DIR"
  fi
  printf '\nThen run:\n  favro-mcp login\n  favro-mcp install\n'
fi
