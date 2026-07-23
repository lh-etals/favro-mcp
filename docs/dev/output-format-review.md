# Output-format review (analysis — no implementation yet)

Goal: make tool outputs **human-readable, navigable, and name-forward**, using
Markdown bodies for messages/details and YAML frontmatter for strictly
structured listings/search results. This doc proposes the contract and shows a
before/after for representative tools. **Nothing is implemented until confirmed.**

## Current state

Every tool returns an indented **JSON blob** via `jsonResult`. Examples today:

```json
// list_boards
{ "boards": [ { "widget_common_id": "255a…", "name": "Tasks", "type": "board",
  "archived": false, "collection_ids": ["2000c6…"] } ], "collection_filter": null }

// list_cards
{ "cards": [ { "card_id": "c7e4…", "sequential_id": 3709, "name": "ZZ-…",
  "column_id": "abc…", "tags": [], "archived": false } ],
  "page": 0, "total_pages": 3, "cards_on_page": 30 }
```

## Findings (gaps)

1. **IDs are primary, names are secondary.** Lists lead with `widget_common_id`,
   `card_id`, `column_id`. An agent/human reads names but must copy IDs to act.
   → Make **name primary, id the stable reference** (`name  · id`).
2. **Missing navigation references.**
   - `list_boards` shows `collection_ids` but not the **collection name** — you
     can't tell which folder a board lives in without a separate call.
   - `list_cards` shows `column_id` but not the **column name**.
   - `get_card_details` returns `assignments[].user_id` and `comments[].user_id`
     as raw IDs — the agent must call `get_user` per ID to read them.
   → Resolve references to **names** where the N is small (board detail, card
     detail); for high-N paginated lists (`list_cards`), keep IDs but add the
     cheap local name (column name is already in the board context).
3. **No search.** `list_cards` filters by column/archived only — no text query.
   → Add optional `query` (case-insensitive substring on card name) for
   client-side search. (Favro has no server-side text search, so this is local.)
4. **Format.** JSON is fine for machines but poor for scanning. → Markdown body
   for messages/details; YAML frontmatter for listings/search (machine-parseable
   AND readable).
5. **Errors** are already structured JSON `{error:{kind,status,message}}` — keep
   as-is (it's the machine contract). Optional: prefix a one-line Markdown hint.

## Proposed format contract

| Tool class | Format |
| --- | --- |
| **List / search** (`list_*`) | YAML frontmatter (rows + meta), then a short Markdown summary line |
| **Detail** (`get_card_details`, `get_board`, `get_user`) | Markdown body: heading (name + #seq), sections; IDs in backticks; sub-entities as MD lists/tables with names resolved |
| **Message** (create/update/move/assign/tag/comment/attachment/delete results) | One Markdown sentence with the salient name + id |
| **Error** | Structured JSON (unchanged) — optionally a 1-line MD hint above it |

Conventions:
- Every entity rendered as: **`Name`** `· id` (name bold, id in backticks).
- Pagination shown in frontmatter (`page`, `pages`) + an MD hint when more pages exist.
- Name-based navigation already works (resolvers accept ID **or** name for
  board/org/column/tag/user; card accepts ID / `#seq` / name-within-board). This
  review just makes lists *surface* the names to use.

## Before / After

### list_boards
**Before** — JSON (IDs primary, collection unnamed).
**After** — YAML frontmatter, name-primary, collection name resolved:
````
---
collection: Internal Tasks (AiApp)  · 2000c67d3c948e5bb2646fd6
boards:
  - Tasks            · 255a5d9c834f8dcf14b732e1   (board, 6 columns, lanes: no)
  - Archive          · 2b66d04e2b1f52442db579f8   (board, 3 columns, lanes: no)
---
2 boards in **Internal Tasks (AiApp)**.
````

### list_cards
**Before** — JSON array with `column_id`.
**After** — frontmatter with column name, optional `query` search:
````
---
board: Tasks  · 255a5d9c834f8dcf14b732e1
page: 1 of 3   (next: page=2)
cards:
  - "#3709 Fix login redirect"  · c7e476131c11a9c14fc28d15   column: In Progress · f2df3…   tags: [bug]   archived: no
  - "#3708 Export CSV"          · 062abbc3884d5818023c6cae   column: Done       · a1bc…     tags: []      archived: no
---
30 cards on this page (page 1/3). Use page=N for more; pass query="…" to filter by name.
````

### get_card_details
**Before** — deep JSON object (`assignments[].user_id`, `comments[].user_id` unresolved).
**After** — Markdown body, references resolved to names:
````
# #3709 Fix login redirect  · c7e476131c11a9c14fc28d15

**Board:** Tasks · column **In Progress** · lane —
**Status:** active · **Due:** 2026-08-01
**Assigned:** Jane Doe (`NR8A…`), Sam Rae (`QzZn…`)
**Tags:** bug, backend

## Description
Steps to reproduce the 500 on `/login` …

## Checklists
- **QA** (2/3)
  - [x] Reproduce locally
  - [x] Add failing test
  - [ ] Fix root cause

## Comments
- **Jane Doe** (2026-07-22): "Looks like a missing redirect header."
- **Sam Rae** (2026-07-23): "+1, seeing it on staging."
````

### get_board
**After** — Markdown body:
````
# Tasks  · 255a5d9c834f8dcf14b732e1

**Collection:** Internal Tasks (AiApp) · **Lanes:** none

## Columns
1. **Backlog** · b01…  (12 cards)
2. **In Progress** · f2d…  (5 cards)
3. **Done** · a1b…  (40 cards)
````

### create_card (message)
**Before** — `{ "message": "Created card #3709: Fix login redirect", "card_id": "c7e4…", ... }`
**After** — Markdown sentence:
````
Created card **#3709 Fix login redirect** (`c7e476131c11a9c14fc28d15`) on **Tasks** in **In Progress**.
````

### move_card (message)
**After:**
````
Moved **#3709 Fix login redirect** to column **Done** (`a1bc…`) on board **Tasks**.
````

### assign_card / tag_card (message)
**After:**
````
Assigned **Jane Doe** to **#3709 Fix login redirect**.
Added tag **bug** to **#3709 Fix login redirect**.
````

### list_users / list_tags (lists)
**After** — frontmatter, name-primary:
````
---
users:
  - Jane Doe       · NR8APM8m8BemMnmaA   jane@acme.com    (administrator)
  - Sam Rae        · QzZnxqiJgd6GrJWKP   sam@acme.com     (member)
---
2 users.
````

### Errors (unchanged contract)
**After** (keep structured JSON; optional MD hint line):
````
Could not find that card.
{"error":{"kind":"not_found","status":0,"message":"card not found: zzz-nonexistent"}}
````

## Decisions needed from you

1. **YAML frontmatter for lists** — confirm, or prefer a Markdown table instead?
2. **Name resolution in `get_card_details`** — resolve `user_id`→name in
   assignments/comments (small N, a few extra API calls). OK?
3. **`query` search param on `list_cards`** (client-side name substring). Add it?
4. **Errors** — keep JSON-only, or add the 1-line Markdown hint above it?
5. **Backward compatibility** — this is a breaking change to all output shapes.
   Pre-release, so acceptable? (No external consumers yet.)

## Implementation sketch (after confirmation; display-layer only)

- Tool/logic/resolvers/tiers **unchanged**.
- Add render helpers: `renderList(meta, rows)`, `renderDetail(...)`,
  `renderMessage(fmt, args…)`.
- Each tool builds its data, then renders via the right helper instead of
  `jsonResult(map…)`.
- New navigation refs require a few extra resolved names (collection name in
  `list_boards`; column name in `list_cards`; user names in `get_card_details`).
- Tests written **after** this lands, against the final format.
```
