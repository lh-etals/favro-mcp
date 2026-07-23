# Favro MCP

A single-binary [MCP](https://modelcontextprotocol.io) server for
[Favro](https://favro.com) project management, plus an installer that detects
your AI clients and registers itself with them.

This is a Go port of [truls27a/favro-mcp](https://github.com/truls27a/favro-mcp).
It exposes the same tools as the Python original, packaged as one ~9 MB static
binary for Linux, macOS, and Windows (x64 + arm64).

Maintained by [Etals](https://etals.com).

---

## Install

**macOS / Linux:**

```bash
curl -fsSL https://github.com/lh-etals/favro-mcp/raw/main/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://github.com/lh-etals/favro-mcp/raw/main/install.ps1 | iex
```

Each downloads the matching binary to `~/.favro-mcp/bin` (or `%LOCALAPPDATA%\favro-mcp`)
and adds it to your PATH. Open a new terminal afterwards so `favro-mcp` is found.

### Getting your Favro API token

1. Log in to [Favro](https://favro.com)
2. Click your **username** (top-left) → **My Profile**
3. Go to **API Tokens** → **Create new token**
4. **Copy the token** — you won't see it again.

### Register with your AI clients

```
favro-mcp install
```

This detects the MCP-capable clients installed on your machine (Claude Desktop,
Claude Code, Cursor, Codex, Gemini CLI, Windsurf, Zed, Cline, Roo Code,
Amazon Q, Continue) and registers the server with the ones you choose. It
prompts for your Favro email + token and writes each client's config safely
(your other servers are preserved). Flags:

- `--dry-run` — show what would change, write nothing
- `--yes` — register with all detected clients, no prompts
- `--name <name>` — server name (default `favro`)
- `--email` / `--token` — provide credentials non-interactively

To remove it later: `favro-mcp uninstall`.

### Manual configuration

If you prefer to edit a client config by hand, point it at the binary:

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

The server reads `FAVRO_EMAIL` and `FAVRO_API_TOKEN` from its environment.

---

## Tools

### Organizations
| Tool | Description |
| --- | --- |
| `list_organizations` | List all organizations |
| `get_current_organization` | Get current organization |
| `set_organization` | Set active organization |

### Collections (Folders)
| Tool | Description |
| --- | --- |
| `list_collections` | List all collections (folders) |

### Boards
| Tool | Description |
| --- | --- |
| `list_boards` | List boards (optionally filter by collection) |
| `get_board` | Get board with columns |
| `get_current_board` | Get current board |
| `set_board` | Set active board |

### Cards
| Tool | Description |
| --- | --- |
| `list_cards` | List cards on board (paginated) |
| `get_card_details` | Get card details (with tasklists + comments) |
| `add_comment` | Add a comment to a card |
| `create_card` | Create a card |
| `update_card` | Update a card |
| `move_card` | Move card to a column/lane, or another board |
| `assign_card` | Assign/unassign a user |
| `tag_card` | Add/remove a tag |
| `delete_card` | Delete a card |
| `list_custom_fields` | List custom fields |
| `upload_attachment` | Upload a file attachment |

### Tags
| Tool | Description |
| --- | --- |
| `list_tags` | List all tags |

### Users
| Tool | Description |
| --- | --- |
| `list_users` | List users in the organization |
| `get_user` | Look up a user by ID, name, or email |

### Columns
| Tool | Description |
| --- | --- |
| `list_columns` | List columns on a board |
| `create_column` | Create a column |
| `rename_column` | Rename a column |
| `move_column` | Move a column's position |
| `delete_column` | Delete a column |

### Lanes (Swimlanes)
| Tool | Description |
| --- | --- |
| `list_lanes` | List lanes (swimlanes) on a board |

Lanes are read-only in the Favro API. To place a card in a lane, pass its ID
or name as the `lane` argument to `create_card`, `update_card`, or `move_card`.

---

## Build from source

Requires [Go 1.23+](https://go.dev/dl/):

```bash
git clone https://github.com/lh-etals/favro-mcp.git
cd favro-mcp
go build -o favro-mcp .
./favro-mcp install
```

Cross-compile every target with `CGO_ENABLED=0`:

```bash
for t in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64; do
  CGO_ENABLED=0 GOOS="${t%-*}" GOARCH="${t#*-}" go build -ldflags="-s -w" -o "favro-mcp-$t" .
done
```

Releases are built by the `release` workflow on tag push (`git tag v0.x && git push --tags`).

## License

MIT. Based on [truls27a/favro-mcp](https://github.com/truls27a/favro-mcp).
