# Tool undo map & live-test blast radius

Every tool classified by what it changes, whether it can be undone with another
tool, and the blast radius if something goes wrong. Use this to test on the live
account safely: prefer read-only tools, snapshot before writes, and only run
destructive tools on throwaway items.

## Read-only (15) — safe, no undo needed

| Tool | Notes |
| --- | --- |
| `list_organizations` | |
| `get_current_organization` | reads session state |
| `list_collections` | |
| `list_boards` | |
| `get_board` | board + columns + lanes |
| `get_current_board` | reads session state |
| `list_cards` | paginated |
| `list_custom_fields` | |
| `get_card_details` | card + tasklists + comments |
| `list_tags` | |
| `list_users` | |
| `get_user` | |
| `list_columns` | |
| `list_lanes` | |

## Session-state (2) — server-only, no Favro data change

| Tool | Undo |
| --- | --- |
| `set_organization` | `set_organization(<previous>)` |
| `set_board` | `set_board(<previous>)` |

## Write — reversible via a tool (7)

| Tool | Changes | Undo via | Blast radius | Caveat |
| --- | --- | --- | --- | --- |
| `create_card` | new card | `delete_card(card_id)` | 1 new card | clean |
| `update_card` | card fields / desc / lane / archive / custom_fields | `update_card` with prior values (snapshot first) | 1 card | `add_tasklist`/`add_task`/`tasks` create tasklists/tasks with **no delete tool** (see gaps) |
| `move_card` | card's column / lane / board | `move_card` back (snapshot prior column/lane/board) | 1 card position | cross-board uses dragMode=move |
| `assign_card` | assignment | `assign_card(user, remove=true)` | 1 assignment | toggle |
| `tag_card` | tag on card | `tag_card(tag, remove=true)` | 1 tag on 1 card | toggle |
| `rename_column` | column name | `rename_column(<prior name>)` | 1 column name | snapshot name |
| `move_column` | column position | `move_column(<prior position>)` | 1 column position | snapshot position |

## Write — no clean tool-undo (5) — manual cleanup or destructive

| Tool | Changes | Undo | Blast radius | Safe-test approach |
| --- | --- | --- | --- | --- |
| `add_comment` | new comment | **none** — `DELETE /comments/{id}` exists in the API but is not exposed | 1 comment | test on a throwaway card; delete comment in Favro UI, or add `delete_comment` |
| `upload_attachment` | file on card | **none** — no remove-attachment tool | 1 attachment | test on a throwaway card; clean up in Favro UI |
| `create_column` | new column | `delete_column` **but it also deletes the column's cards** | 1 column (+ cards if non-empty when deleted) | only undo via `delete_column` while the column is still empty |
| `delete_card` | destroys a card | **none — irreversible** | card gone | run only on a card you created for the test |
| `delete_column` | destroys column + its cards | **none — irreversible** | column + cards gone | run only on a throwaway column |

## Undo gaps (API supports, our tools don't)

Implementing these closes most of the "no clean undo" rows above, so every
write becomes reversible:

- `delete_comment` → `DELETE /comments/{commentId}` (undoes `add_comment`)
- `delete_task` → `DELETE /tasks/{taskId}` (undoes `update_card`'s `add_task`)
- `delete_tasklist` → `DELETE /tasklists/{taskListId}` (undoes `update_card`'s `add_tasklist`)
- `remove_attachment` → `DELETE /cards/{cardId}/attachment/{attachmentId}` (undoes `upload_attachment`)

## Recommended live-test order

1. Read-only tools — exercise freely (these also seed the API recorder fixtures).
2. Create a throwaway board + card on a test collection; snapshot their IDs.
3. Reversible writes (`update_card`, `move_card`, `assign_card`, `tag_card`,
   `rename_column`, `move_column`) — snapshot before, revert after.
4. `create_card` then `delete_card` (round-trip on throwaway).
5. `add_comment` / `upload_attachment` — on the throwaway card, then clean up
   manually (or after we add the delete tools above).
6. `delete_column` — last, on the empty throwaway column.
