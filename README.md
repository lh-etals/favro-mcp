# Favro MCP

A single-binary [MCP](https://modelcontextprotocol.io) server for
[Favro](https://favro.com) — rewritten in Go for speed, portability, and a
zero-dependency install.

One static **~9 MB** binary. No Python, no `uv`, no `pip`, no Node. It runs
on Linux, macOS, and Windows (x64 + arm64), **auto-detects which AI clients you
have installed**, and registers itself with the ones you pick.

- **28 MCP tools** across Organizations, Collections, Boards, Cards, Tags,
  Users, Columns, and Lanes
- **Universal one-command installers** — `curl | sh` on macOS/Linux,
  `irm | iex` on Windows
- **Auto-registers** with Claude Desktop, Claude Code, Cursor, Codex, Gemini
  CLI, Windsurf, Zed, Cline, Roo Code, Amazon Q, Continue, OpenCode, and VS Code
- **Secure by design** — your Favro token lives in your local client config and
  talks straight to the Favro API. No proxy, no third-party service.

A Go port of [truls27a/favro-mcp](https://github.com/truls27a/favro-mcp)
(same 28 tools, same Favro API). Maintained by [Etals](https://etals.com).

---

## Install

**macOS / Linux** — downloads the matching binary to `~/.favro-mcp/bin` and
adds it to your `PATH`:

```bash
curl -fsSL https://github.com/lh-etals/favro-mcp/raw/main/install.sh | sh
```

**Windows (PowerShell)** — downloads to `%LOCALAPPDATA%\favro-mcp` and adds it
to your user `PATH`:

```powershell
irm https://github.com/lh-etals/favro-mcp/raw/main/install.ps1 | iex
```

Each installer picks the right asset for your OS/arch
(`favro-mcp-<os>-<arch>[.exe]`) from the latest
[GitHub Release](https://github.com/lh-etals/favro-mcp/releases). **Open a new
terminal** afterward so `favro-mcp` is found.

### Get your Favro API token

1. Log in to [Favro](https://favro.com)
2. Click your **username** (top-left) → **My Profile**
3. Go to **API Tokens** → **Create new token**
4. **Copy the token** — you won't see it again.

### Log in

```
favro-mcp login
```

Prompts for your Favro email and API token (the token is hidden while you
paste it), stores them in `~/.favro-mcp/credentials.json` (mode `0600`), and
verifies them against the Favro API. The server reads this file at startup, so
your credentials live in one place instead of being copied into every client
config. You only do this once.

> Don't want a credential file? See **Manual configuration** below — you can put
> `FAVRO_EMAIL`/`FAVRO_API_TOKEN` straight in a client's `env` block and skip
> `login` entirely; the server honours those env vars (they take precedence over
> the stored credentials).

### Register with your AI clients

```
favro-mcp install
```

This scans your machine for MCP-capable clients (Claude Desktop, Claude Code,
Cursor, Codex, Gemini CLI, Windsurf, Zed, Cline, Roo Code, Amazon Q, Continue),
and lets you pick which to wire up with an interactive checklist. Each client's
config is written **safely and idempotently** — your other servers are
preserved, and re-running won't duplicate anything. The one-line installer
above runs `login` then `install` for you automatically.

Flags:

| Flag | What it does |
| --- | --- |
| `--dry-run` | Show exactly what would change, write nothing |
| `--yes` | Register with **all** detected clients, no prompts |
| `--name <name>` | Server name written into configs (default `favro`) |
| `--toolset <t>` | Toolset to expose: `read`, `write`, `delete`, or `custom` |
| `--email <addr>` | Embed this email into configs (instead of using `login`) |
| `--token <tok>` | Embed this token into configs (instead of using `login`) |

Remove it everywhere later with `favro-mcp uninstall`.

### Toolsets

`install` asks which tools the server should expose (set permanently via
`FAVRO_TOOLSET` / `FAVRO_TOOLS` in each client's config):

- **Read-only** — list/get only. Cannot change anything. Safest for giving an
  agent read access to your boards.
- **Read + Write** *(default)* — also create/update/move cards, columns, tags,
  comments, attachments. No deletes.
- **Read + Write + Delete** — full access, including deletes.
- **Custom** — a toggle view of every individual tool; pick exactly which ones
  the agent may call.

### Manual configuration

Prefer to edit a client config by hand? Point it at the binary. Credentials in
the config's `env` block are read directly by the server (and override any
stored login):

```json
{
  "mcpServers": {
    "favro": {
      "command": "/absolute/path/to/favro-mcp",
      "env": {
        "FAVRO_EMAIL": "your-email@example.com",
        "FAVRO_API_TOKEN": "your-token-here"
      }
    }
  }
}
```

Credential resolution order: **`env` block (or `FAVRO_EMAIL`/`FAVRO_API_TOKEN`
environment variables) → `favro-mcp login` store**. If neither is present the
server returns a clear error on first use.

---

## Favro MCP distributions

There's more than one way to run Favro from an AI agent. Here are the known
MCP distributions for Favro:

- **This repo — `lh-etals/favro-mcp` (Go).**
  A single static binary (~9 MB) with a universal `curl | sh` / `irm | iex`
  installer and built-in auto-detection of 13 AI clients. 32 tools across
  read / write / delete tiers (plus per-tool custom). No runtime dependencies.
  [github.com/lh-etals/favro-mcp](https://github.com/lh-etals/favro-mcp) ·
  [releases](https://github.com/lh-etals/favro-mcp/releases)
- **`truls27a/favro-mcp` (Python).**
  The original Favro MCP server, installed via `uv`/`pip` (`uvx favro-mcp`).
  This Go repo forked from it, kept feature parity with the same Favro API
  surface, and rewrote the server in Go with an installer on top.
  [github.com/truls27a/favro-mcp](https://github.com/truls27a/favro-mcp)

---

## Tools

All **28 tools** use the Favro REST API. IDs, names, or emails are accepted
wherever a Favro object is referenced.

### Organizations
| Tool | Description |
| --- | --- |
| `list_organizations` | List all organizations you can access |
| `get_current_organization` | Get the currently active organization |
| `set_organization` | Set the active organization (by ID or name) |

### Collections (Folders)
| Tool | Description |
| --- | --- |
| `list_collections` | List all collections (folders containing boards) |

### Boards
| Tool | Description |
| --- | --- |
| `list_boards` | List boards (optionally filter by collection) |
| `get_board` | Get a board with its columns and lanes |
| `get_current_board` | Get the currently active board |
| `set_board` | Set the active board (by ID or name) |

### Cards
| Tool | Description |
| --- | --- |
| `list_cards` | List cards on a board (paginated, 100 per page) |
| `get_card_details` | Full card: description, assignments, dates, custom fields, tasklists, comments |
| `create_card` | Create a card (markdown desc, tags, assignees optional) |
| `update_card` | Update a card's name, description, lane, archive state, custom fields, tasks |
| `move_card` | Move a card to a column/lane, or to another board |
| `delete_card` | Delete a card (`everywhere=true` removes it from all boards) |
| `assign_card` | Assign / unassign a user (by ID, name, or email) |
| `tag_card` | Add / remove a tag (by ID or name) |
| `add_comment` | Add a comment to a card |
| `upload_attachment` | Upload a file attachment (max 10 MB) to a card |
| `list_custom_fields` | List custom-field definitions for `update_card` |

### Tags
| Tool | Description |
| --- | --- |
| `list_tags` | List all tags (IDs, names, colors) |

### Users
| Tool | Description |
| --- | --- |
| `list_users` | List users in the organization |
| `get_user` | Look up a user by ID, name, or email |

### Columns
| Tool | Description |
| --- | --- |
| `list_columns` | List a board's columns (sorted by position) |
| `create_column` | Create a column (appends unless a position is given) |
| `rename_column` | Rename a column (by column ID or name) |
| `move_column` | Move a column to a new 0-based position |
| `delete_column` | Delete a column (**and all its cards**) |

### Lanes (Swimlanes)
| Tool | Description |
| --- | --- |
| `list_lanes` | List a board's lanes (swimlanes) |

Lanes are read-only in the Favro API — they can't be created, renamed, or
deleted. To place a card in a lane, pass the lane ID or name as the `lane`
argument to `create_card`, `update_card`, or `move_card`.

---

## Build from source

Requires [Go 1.25+](https://go.dev/dl/):

```bash
git clone https://github.com/lh-etals/favro-mcp.git
cd favro-mcp
go build -o favro-mcp .
./favro-mcp install
```

Cross-compile all six targets with `CGO_ENABLED=0`:

```bash
for t in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64; do
  CGO_ENABLED=0 GOOS="${t%-*}" GOARCH="${t#*-}" go build -ldflags="-s -w" -o "favro-mcp-$t" .
done
```

Releases are produced by the `release` workflow on tag push
(`git tag v0.x && git push --tags`).

---

## License

MIT. Based on [truls27a/favro-mcp](https://github.com/truls27a/favro-mcp) by
Truls Borgvall. Maintained by [Etals](https://etals.com).
