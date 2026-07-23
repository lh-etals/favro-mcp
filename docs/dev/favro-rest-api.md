# Favro REST API Reference (compiled)

> **Status of this document.** Favro's official developer documentation at
> <https://favro.com/developer/> is delivered as a single-page JavaScript
> application (Meteor) whose content is fetched dynamically via DDP and cannot be
> retrieved statically with an HTTP client. Every attempt to scrape it returned
> only the `Initializing…` loader shell, and no snapshots exist in the Internet
> Archive. This file was therefore **compiled by cross-referencing multiple
> independent, source-of-truth artifacts**, then **verified against the live
> API** (see "How verified" below). Where the official SPA remains the only
> source for a detail, it is cited inline and the section is flagged as
> *partial*.

## Sources

| Source | URL | Used for |
| --- | --- | --- |
| Official developer docs (SPA) | <https://favro.com/developer/> | Canonical endpoint names / fragment anchors (`#get-all-cards`, …) referenced by the libraries below |
| Bravo SDK — `bscotch/favro-sdk` (TypeScript) | <https://github.com/bscotch/favro-sdk> | Complete data-model typings, webhook & group endpoints, custom-field value shapes |
| Favro MCP (Python original) — `truls27a/favro-mcp` | <https://github.com/truls27a/favro-mcp> | Endpoint surface used by the reference MCP server |
| PHP Favro API wrapper — `seregazhuk/php-favro-api` | <https://github.com/seregazhuk/php-favro-api> | Cross-check of resource list |
| This project's Go client — `internal/favro` | `internal/favro/client.go`, `endpoints.go`, `models.go` | Behavior of the server this doc ships with |
| Live API probe | `GET https://favro.com/api/v1/<res>` with bad creds | Existence check (400 = route exists, 404 = route absent) |

### How "verified" works

With no/invalid credentials, a **real** Favro route answers `HTTP 400` (bad
request — the route matched but the request is malformed / unauthorized),
whereas a **non-existent** route answers `HTTP 404`. The probe result is
recorded per resource in [Appendix B — Resource existence probe](#appendix-b--resource-existence-probe).

### Completeness summary

| Area | Completeness |
| --- | --- |
| Base URL, auth, headers, rate-limit, pagination, error envelope | **Complete** |
| Organizations, Users, Collections, Widgets, Columns, Cards, Tags, Comments, Custom fields, Task lists, Tasks | **Complete** (method/path/params/fields) |
| Groups, Webhooks (outgoing) | **Complete** (from Bravo typings + live probe) |
| Lanes | **Complete** — documented as *embedded in Widget*, no dedicated route (confirmed 404) |
| Hypothesized resources (contacts, relations, expenses, schedulers, calendar, subtasks, activities) | **Confirmed absent** — all return 404 (see Appendix B) |
| Webhook event payloads, custom-field *value* update shapes | **Complete** (Bravo typings) |
| Exact numeric rate-limit quota / window | **Partial** — only header names known; official SPA has the numbers |

---

## 1. Conventions

### Base URL

```
https://favro.com/api/v1
```

All paths below are relative to this base.

### Authentication

HTTP **Basic** auth with the Favro user's **email** as the username and an
**API token** as the password. Tokens are created in Favro under
*username → My Profile → API Tokens → Create new token*.

```
Authorization: Basic <base64(email:token)>
```

### Required / common headers

| Header | When | Notes |
| --- | --- | --- |
| `Authorization` | Every request | Basic, as above. |
| `organizationId` | Almost every request | The org to operate in. **Omit** for `/organizations` and `/users/{id}` lookups (they are user-scoped, not org-scoped). |
| `Content-Type` | Write requests | `application/json` for JSON bodies; `application/octet-stream` for binary attachment uploads. |
| `Host: favro.com` | All requests | Required by the API gateway (requests without it fail opaquely). |
| `X-Favro-Backend-Identifier` | All requests after the first | Echoed back by the server in the **response** header of the same name. Sticky-routing hint — capture it from the first response and resend it on subsequent requests for session affinity. Optional but recommended. |
| `User-Agent` | Optional | Set something identifiable; some clients set `BravoClient <…>`. |

> **`organizationId` gotcha.** Endpoints that are global to the user
> (`GET /organizations`, `GET /users/{id}`) must be called **without** the
> `organizationId` header; sending it can cause errors. All org-scoped
> endpoints require it. Our client models this with an `includeOrg` bool per
> call (`client.go:55`).

### Rate limiting

Favro throttles per-token. The relevant response headers:

| Header | Meaning |
| --- | --- |
| `X-RateLimit-Remaining` | Requests left in the current window. |
| `X-RateLimit-Reset` | Timestamp (epoch, UTC) when the window resets. |
| `X-RateLimit-Limit` *(observed)* | The window's request budget. |

On exhaustion the API returns **`HTTP 429`** with `X-RateLimit-Reset` set. Our
client surfaces this as a `RateLimitError` carrying `ResetTime`
(`client.go:29`). The exact numeric quota/window is documented only in the
official SPA and is therefore **partial** here.

### Pagination model

List endpoints return the standardized paged envelope:

```jsonc
{
  "entities": [ /* one page of records */ ],
  "page": 0,            // current page (0-indexed)
  "pages": 3,           // total page count
  "limit": 1000,        // page size
  "requestId": "abcd",  // stable id for this result set
  "message": "..."      // only present on some error-shaped 200s
}
```

To fetch further pages, **re-issue the same request** adding:

| Param | Value |
| --- | --- |
| `requestId` | The `requestId` from the first response. |
| `page` | 1, 2, … (`pages`-1). 0-indexed. |

Notes:
- The first page does **not** require `requestId`; subsequent pages do.
- `requestId` is required to keep the result set stable while paging.
- Some endpoints (e.g. `GET /customfields`) support only page-by-page fetching,
  not query filters.
- Our client's `paginateAll` / `paginateSingle` implement this exactly
  (`client.go:181`, `client.go:212`).

### Response envelope & error handling

- **Success with body:** the resource object (single) or the paged envelope (list).
- **`204 No Content`:** successful delete / no-body response.
- **Status `200` + `{"message": "…"}` (only key):** an error masquerading as
  success; treat the `message` as an error string (our client does, `client.go:118`).

| Status | Meaning | Our error type |
| --- | --- | --- |
| `400` | Bad request / malformed params / unauthorized-but-route-exists | `APIError` |
| `401` | Invalid credentials | `AuthError` |
| `403` | Access denied / forbidden | `AuthError` |
| `404` | Resource (or route) not found | `NotFoundError` |
| `429` | Rate limit exceeded (see `X-RateLimit-Reset`) | `RateLimitError` |
| `5xx` | Server error — some card endpoints return `500` when `descriptionFormat=markdown` is sent; retry without it | `APIError` |

> **Markdown quirk.** Passing `descriptionFormat=markdown` on card requests can
  trigger `HTTP 500` on some cards. Our client (and the Python original)
  transparently retries the call without that param when a 500 is received
  (`endpoints.go:184`).

---

## 2. Resources

### 2.1 Organizations

The top-level unit. A token works across all of the user's organizations; pick
one by setting the `organizationId` header on subsequent calls.

| Method | Path | Purpose | Key params | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/organizations` | List all orgs the user belongs to | — (omit `organizationId` header) | `entities[]`: `organizationId`, `name`, `sharedToUsers[]` (`{userId, role, joinDate}`) |
| `GET` | `/organizations/{organizationId}` | Get one org | — | same fields as above |

Roles on `sharedToUsers[].role`: `administrator | fullMember | externalMember | guest | disabled`.

*No create/update/delete org endpoints exist via the public REST API.*

Used by our code: `GetOrganizations`, `GetOrganization` (`endpoints.go:28`,`36`).

### 2.2 Users

| Method | Path | Purpose | Key params | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/users` | List all users in the current org | (paginated; `organizationId` header required) | `entities[]`: `userId`, `name`, `email`, `organizationRole` |
| `GET` | `/users/{userId}` | Get one user | — (omit `organizationId` header) | same fields |

`organizationRole`: `administrator | fullMember | externalMember | guest | disabled`.

*No create/update/delete user endpoints exist via the public REST API.*

Used by our code: `GetUsers`, `GetUser` (`endpoints.go:18`,`10`).

### 2.3 Groups

User groups within an organization (used for assigning cards to a group — see
`Card.assignment.isGroup`).

| Method | Path | Purpose | Key params / body | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/groups` | List all groups | (paginated) | `entities[]`: see model |
| `GET` | `/groups/{groupId}` | Get one group | — | same |
| `POST` | `/groups` | Create a group | `{name, members:[{userId|email, role}]}` | created group |
| `PUT` | `/groups/{groupId}` | Update a group | `{name?, members:[{userId|email, delete?|role}]}` | updated group |
| `DELETE` | `/groups/{groupId}` | Delete a group | — | `204` |

**Group model**
```jsonc
{
  "groupId": "...",
  "organizationId": "...",
  "name": "...",
  "creatorUserId": "...",
  "memberCount": 3,
  "members": [{ "userId": "...", "role": "administrator|member" }]
}
```

> **Not used by our code.** Adding card-assignment-to-group support would
> benefit from group lookup tools.

### 2.4 Collections (folders)

| Method | Path | Purpose | Key params / body | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/collections` | List collections | `archived=true|false` | `entities[]`: see model |
| `GET` | `/collections/{collectionId}` | Get one collection | — | same |
| `POST` | `/collections` | Create a collection | `{name, publicSharing?, background?, sharedToUsers?}` | created collection |
| `DELETE` | `/collections/{collectionId}` | Delete a collection | — | `204` |
| `PUT` | `/collections/{collectionId}` | Update a collection | *(partial; SPA-only detail)* | updated collection |

**Collection model**
```jsonc
{
  "collectionId": "...",
  "organizationId": "...",
  "name": "...",
  "sharedToUsers": [{ "userId": "...", "role": "guest|view|edit|admin" }],
  "publicSharing": "users|organization|public",
  "background": "purple|green|grape|red|pink|blue|solidPurple|solidGreen|solidGrape|solidRed|solidPink|solidGray",
  "archived": false,
  "fullMembersCanAddWidgets": true
}
```

> **Create/Delete/Update collection are not used by our code** (we only read).

### 2.5 Widgets (boards)

A "widget" is a board. `widgetCommonId` is the shared id used everywhere else.

| Method | Path | Purpose | Key params / body | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/widgets` | List widgets | `collectionId?`, `archived=true|false` | `entities[]`: see model |
| `GET` | `/widgets/{widgetCommonId}` | Get one widget (includes `lanes[]` if lanes enabled) | — | same, plus `lanes` |
| `POST` | `/widgets` | Create a widget | `{collectionId, name, type?, color?}` | created widget |
| `DELETE` | `/widgets/{widgetCommonId}` | Delete a widget | — | `204` |
| `PUT` | `/widgets/{widgetCommonId}` | Update a widget | *(partial; SPA-only detail)* | updated widget |

**Widget model**
```jsonc
{
  "widgetCommonId": "...",
  "organizationId": "...",
  "collectionIds": ["..."],
  "name": "...",
  "type": "backlog|board",
  "color": "blue|lightgreen|brown|purple|orange|yellow|gray|red|cyan|green",
  "ownerRole": "owners|fullMembers|guests",
  "editRole":  "owners|fullMembers|guests",
  "archived": false,
  "breakdownCardCommonId": "...",   // set iff this widget is a card breakdown
  "lanes": [{ "laneId": "...", "name": "..." }]   // only if lanes enabled
}
```

> **Create/Delete/Update widget are not used by our code** (we only read).
> **Lanes** are returned embedded here — there is **no** `/lanes` route
> (confirmed 404). See §2.6.

### 2.6 Lanes (swimlanes)

Lanes are **read-only** and returned **nested inside the Widget object**
(`widget.lanes[]`). There is no dedicated collection/route for lanes and the
Favro API exposes no create/rename/delete for them. To place a card in a lane,
send `laneId` on card create/update/move.

| Method | Path | Purpose | Notes |
| --- | --- | --- | --- |
| `GET` | `/widgets/{widgetCommonId}` | Read lanes via the parent widget | `lanes[]: {laneId, name}` |

Used by our code: `GetLanes` reads them off the widget (`endpoints.go:85`).

### 2.7 Columns

| Method | Path | Purpose | Key params / body | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/columns` | List columns on a widget | `widgetCommonId` | `entities[]`: see model |
| `GET` | `/columns/{columnId}` | Get one column | — | same |
| `POST` | `/columns` | Create a column | `{widgetCommonId, name, position?}` | created column |
| `PUT` | `/columns/{columnId}` | Update (rename/move) a column | `{name?, position?}` | updated column |
| `DELETE` | `/columns/{columnId}` | Delete a column | — | `204` |

**Column model**
```jsonc
{
  "columnId": "...",
  "organizationId": "...",
  "widgetCommonId": "...",
  "name": "...",
  "position": 1024,
  "cardCount": 12,
  "timeSum": 3600000,        // ms spent by cards in this column (nullable)
  "estimationSum": 8         // nullable
}
```

Used by our code: full CRUD (`endpoints.go:98`–`145`).
*Caveat:* the API forbids deleting the **last** column on a widget (returns 403).

### 2.8 Cards

Cards are the central work unit. A card has a per-widget `cardId` and an
org-global `cardCommonId`. Most fields are common across a card's instances.

#### Endpoints

| Method | Path | Purpose | Key params | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/cards` | List cards (paginated) | `widgetCommonId?`, `collectionId?`, `columnId?`, `cardSequentialId?`, `cardCommonId?`, `todoList=true`, `unique=true`, `archived=true|false`, `descriptionFormat=plaintext\|markdown` | `entities[]`: see model |
| `GET` | `/cards/{cardId}` | Get one card (by per-widget id) | `descriptionFormat=plaintext\|markdown` | single card |
| `POST` | `/cards` | Create a card | body (see Create) | created card |
| `PUT` | `/cards/{cardId}` | Update a card | body (see Update) | updated card |
| `DELETE` | `/cards/{cardId}` | Delete a card | `everywhere=true` deletes on **all** boards (default: only the board of this `cardId`) | `204` |

**`GET /cards` filters** — at least one of `cardCommonId`, `collectionId`,
`widgetCommonId`, or `cardSequentialId` should be provided. `unique=true`
returns one record per `cardCommonId` (dedupes across boards). `cardSequentialId`
is the human-visible number (e.g. `123` for `#123`).

**Create body**
```jsonc
{
  "name": "Card title",                 // required
  "widgetCommonId": "...",              // board to post on
  "columnId": "...",
  "laneId": "...",                      // only if widget has lanes
  "detailedDescription": "...",
  "descriptionFormat": "plaintext|markdown",
  "addTags": ["bug"],                   // creates tag if missing
  "startDate": "2024-01-01T00:00:00.000Z",
  "dueDate":   "2024-01-31T00:00:00.000Z",
  "addAssignmentIds": ["userId|groupId"]
}
```

**Update body** (all fields optional)
```jsonc
{
  "name": "...",
  "detailedDescription": "...",
  "widgetCommonId": "...",        // target board (combine with dragMode)
  "columnId": "...",
  "laneId": "...",
  "dragMode": "commit|move",      // 'commit' = copy onto target board (default);
                                  //   'move'   = relocate instance off source board
  "listPosition": 1024.0,         // kanban / todo-list order
  "sheetPosition": 1024.0,        // hierarchical (sheet/card-list) order
  "parentCardId": "...",          // parent in card hierarchy
  "addAssignmentIds": [...], "removeAssignmentIds": [...],
  "completeAssignments": [{ "userId": "...", "completed": true }],
  "addTags": [...], "addTagIds": [...], "removeTags": [...], "removeTagIds": [...],
  "startDate": "...|null",        // null clears the field
  "dueDate":   "...|null",
  "addTasklists": [{ "name": "...", "tasks": ["t1", {"name":"t2","completed":true}] }],
  "removeAttachments": ["fileURL"],
  "customFields": [ /* see §2.12 update shapes */ ],
  "addFavroAttachments":     [{ "itemCommonId": "cardCommonId|widgetCommonId", "type": "card|board|backlog" }],
  "removeFavroAttachmentIds": ["itemCommonId"],
  "archive": true
}
```

**Card model** (selected fields)
```jsonc
{
  "cardId": "...", "organizationId": "...", "cardCommonId": "...",
  "name": "...", "sequentialId": 123,
  "widgetCommonId": "...",         // absent if in a todo list
  "columnId": "...", "laneId": "..."|null, "parentCardId": "..."|null,
  "isLane": false, "archived": false,
  "detailedDescription": "...",
  "tags": ["bug"],
  "startDate": "..."|null, "dueDate": "..."|null,
  "assignments": [{ "userId": "...", "completed": false, "isGroup": false }],
  "numComments": 2, "tasksTotal": 4, "tasksDone": 1,
  "attachments":          [{ "name": "...", "fileURL": "...", "thumbnailURL": "..." }],
  "favroAttachments":     [{ "itemCommonId": "...", "type": "card|board|backlog" }],
  "customFields":         [ /* see §2.12 */ ],
  "timeOnBoard":          { "time": 3600000, "isStopped": false },
  "timeOnColumns":        { "<columnId>": 1200000 },
  "todoListUserId": "..."|undefined,
  "todoListCompleted": false|undefined,
  "listPosition": 1024.0, "sheetPosition": 1024.0,
  "position": 1024.0        // deprecated -> listPosition/sheetPosition
}
```

Used by our code: list (paginated + single page), get, create, update, delete
(`endpoints.go:149`–`381`).

### 2.9 Card attachments

Upload a binary file to a card.

| Method | Path | Purpose | Key params | Response |
| --- | --- | --- | --- | --- |
| `POST` | `/cards/{cardId}/attachment` | Upload an attachment | query `filename`; body = raw bytes; `Content-Type: application/octet-stream` | `{name, fileURL, thumbnailURL}` |

Limits: **10 MB** max (enforced client-side by us, `endpoints.go:386`).
There is **no** top-level `/attachments` route (confirmed 404). Attachment
removal is done via `PUT /cards/{cardId}` with `removeAttachments: [fileURL]`.

Used by our code: `UploadAttachment` (`endpoints.go:385`).

### 2.10 Comments

Comments live on cards (keyed by `cardCommonId`) but are managed via their own
routes and can themselves carry attachments.

| Method | Path | Purpose | Key params / body | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/comments` | List comments on a card | `cardCommonId` | `entities[]`: see model |
| `POST` | `/comments` | Create a comment | `{cardCommonId, comment}` | created comment |
| `PUT` | `/comments/{commentId}` | Edit a comment | `{comment}` | updated comment |
| `DELETE` | `/comments/{commentId}` | Delete a comment | — | `204` |
| `POST` | `/comments/{commentId}/attachment` | Add attachment to a comment | `filename` + binary body | `{name, fileURL, thumbnailURL}` |

**Comment model**
```jsonc
{
  "commentId": "...",
  "organizationId": "...",
  "cardCommonId": "...",
  "userId": "...",
  "comment": "...",
  "created": "2024-01-01T00:00:00.000Z",
  "lastUpdated": "...",                // only if edited
  "attachments": [{ "name": "...", "fileURL": "...", "thumbnailURL": "..." }]
}
```

Used by our code: list + create only (`endpoints.go:417`,`426`). **Edit, delete,
and attachment upload are not implemented.**

### 2.11 Tags

Tags are global to an organization.

| Method | Path | Purpose | Key params / body | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/tags` | List all tags | (paginated) | `entities[]`: see model |
| `GET` | `/tags/{tagId}` | Get one tag | — | same |
| `POST` | `/tags` | Create a tag | `{name, color?}` | created tag |
| `PUT` | `/tags` | Update a tag | `{tagId, name?, color?}` (whole-object PUT) | updated tag |
| `DELETE` | `/tags/{tagId}` | Delete a tag | — | `204` |

**Tag model**
```jsonc
{
  "tagId": "...",
  "organizationId": "...",
  "name": "...",
  "color": "blue|purple|cyan|green|lightgreen|yellow|orange|red|brown|gray|slategray"
}
```

> **Note:** update is a `PUT /tags` (body carries `tagId`), not `PUT /tags/{id}`.

Used by our code: list + get only (`endpoints.go:399`,`407`). **Create/Update/
Delete tag are not implemented.**

### 2.12 Custom fields

Custom field **definitions** are org-wide; their **values** are set per-card.

#### Definitions

| Method | Path | Purpose | Key params | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/customfields` | List all custom-field definitions | (paginated; no filters) | `entities[]`: see model |

> The public REST API exposes **only listing** for definitions — no
> create/update/delete (confirmed by Bravo roadmap & absence of routes).

**Definition model**
```jsonc
{
  "organizationId": "...",
  "customFieldId": "...",
  "type": "Checkbox|Date|Link|Members|Multiple select|Number|Rating|Single select|Tags|Text|Time|Timeline|Voting",
  "name": "...",
  "enabled": true,
  "customFieldItems": [{ "customFieldItemId": "...", "name": "..." }]  // only for Single/Multiple select
}
```

#### Values (set via `PUT /cards/{cardId}` → `customFields[]`)

Each entry has `customFieldId` plus a type-specific value object:

| Field type | Update shape |
| --- | --- |
| Text | `{customFieldId, value: "…"}` |
| Number | `{customFieldId, total: 5}` |
| Rating | `{customFieldId, total: 0..5}` |
| Checkbox | `{customFieldId, value: true}` |
| Date | `{customFieldId, value: "2024-01-15"}` |
| Link | `{customFieldId, link: {url, text}}` |
| Timeline | `{customFieldId, timeline: {startDate, dueDate, showTime}}` |
| Single / Multiple select | `{customFieldId, value: ["customFieldItemId", …]}` |
| Members | `{customFieldId, members: {addUserIds:[…], removeUserIds:[…], completeUsers:[{userId, completed}]}}` |
| Tags | `{customFieldId, tags: {addTags:[…], addTagIds:[…], removeTags:[…], removeTagIds:[…]}}` |
| Voting | `{customFieldId, value: true}` *(vote/unvote)* |
| Time | *(Enterprise; per-user time reports — `{reportId, value(ms), description, createdAt}`)* |

Used by our code: list definitions only, returned as raw maps
(`endpoints.go:437`). Values are writable via `UpdateCardOpts.CustomFields`
(`endpoints.go:317`).

### 2.13 Task lists

Task lists live on a card.

| Method | Path | Purpose | Key params / body | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/tasklists` | List task lists on a card | `cardCommonId` | `entities[]`: see model |
| `POST` | `/tasklists` | Create a task list | `{cardCommonId, name, position?}` | created task list |
| `PUT` | `/tasklists/{taskListId}` | Update a task list | `{name?, position?}` | updated task list |
| `DELETE` | `/tasklists/{taskListId}` | Delete a task list | — | `204` |

**Task list model**
```jsonc
{ "taskListId":"...", "organizationId":"...", "cardCommonId":"...", "name":"...", "position":1024.0 }
```

Used by our code: list + create only (`endpoints.go:443`,`452`).
**Update/Delete task list not implemented.**

### 2.14 Tasks

Tasks live inside a task list.

| Method | Path | Purpose | Key params / body | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/tasks` | List tasks | `cardCommonId`, `taskListId?` | `entities[]`: see model |
| `POST` | `/tasks` | Create a task | `{taskListId, name, position?}` | created task |
| `PUT` | `/tasks/{taskId}` | Update a task | `{name?, completed?, position?}` | updated task |
| `DELETE` | `/tasks/{taskId}` | Delete a task | — | `204` |

**Task model**
```jsonc
{ "taskId":"...", "taskListId":"...", "organizationId":"...", "cardCommonId":"...", "name":"...", "completed":false, "position":1024.0 }
```

Used by our code: list, create, update (`endpoints.go:466`–`505`).
**Delete task not implemented.**

### 2.15 Webhooks (outgoing)

Outgoing webhooks let Favro POST card/comment events to an external URL. They
are attached to a **widget** and filtered by **columnIds** and event types.

#### Management endpoints

| Method | Path | Purpose | Key params / body | Notable response fields |
| --- | --- | --- | --- | --- |
| `GET` | `/webhooks` | List webhooks | `widgetCommonId?` | array: see model |
| `POST` | `/webhooks` | Create a webhook | `{widgetCommonId, name, postToUrl, secret, options}` | created webhook (+ sends a *ping* to `postToUrl`) |
| `DELETE` | `/webhooks/{webhookId}` | Delete a webhook | — | `204` |

**Webhook model**
```jsonc
{
  "webhookId": "...",
  "widgetCommonId": "...",
  "name": "...",
  "postToUrl": "https://example.com/hook",
  "secret": "...",                       // signs the X-Favro-Webhook header
  "options": {
    "columnIds": ["..."],                // which columns trigger notifications (empty = all)
    "notifications": [                    // which events; empty = all
      "Card created","Card committed","Card moved","Card updated","Card removed",
      "Comment added","Comment changed","Comment removed"
    ]
  }
}
```

#### Event payloads (delivered to `postToUrl`)

Common fields: `payloadId`, `action`, `card`, `sender`. Card events also include
`widget`; specific events add context:
- **Card created** → `+ column`
- **Card committed** → `+ column, sourceWidget`
- **Card moved** → `+ column, sourceColumn`
- **Card updated** / **Card removed** → `+ (no extras)`
- **Comment added/changed/removed** → `+ comment`

The receiver **must** answer `HTTP 200`. The `X-Favro-Webhook` request header
is a signature of the body using `secret`; verify it to authenticate the call.

> **Not used by our code at all.** The biggest functional gap — enabling
> real-time/reactive Favro integration.

---

## Appendix A — One-line endpoint index

```
GET    /organizations
GET    /organizations/{organizationId}
GET    /users
GET    /users/{userId}
GET    /groups
GET    /groups/{groupId}
POST   /groups
PUT    /groups/{groupId}
DELETE /groups/{groupId}
GET    /collections
GET    /collections/{collectionId}
POST   /collections
PUT    /collections/{collectionId}
DELETE /collections/{collectionId}
GET    /widgets
GET    /widgets/{widgetCommonId}
POST   /widgets
PUT    /widgets/{widgetCommonId}
DELETE /widgets/{widgetCommonId}
GET    /columns
GET    /columns/{columnId}
POST   /columns
PUT    /columns/{columnId}
DELETE /columns/{columnId}
GET    /cards
GET    /cards/{cardId}
POST   /cards
PUT    /cards/{cardId}
DELETE /cards/{cardId}            (?everywhere=true)
POST   /cards/{cardId}/attachment
GET    /comments
POST   /comments
PUT    /comments/{commentId}
DELETE /comments/{commentId}
POST   /comments/{commentId}/attachment
GET    /tags
GET    /tags/{tagId}
POST   /tags
PUT    /tags                       (body carries tagId)
DELETE /tags/{tagId}
GET    /customfields
GET    /tasklists
POST   /tasklists
PUT    /tasklists/{taskListId}
DELETE /tasklists/{taskListId}
GET    /tasks
POST   /tasks
PUT    /tasks/{taskId}
DELETE /tasks/{taskId}
GET    /webhooks
POST   /webhooks
DELETE /webhooks/{webhookId}
```

## Appendix B — Resource existence probe

`GET https://favro.com/api/v1/<res>` with invalid Basic auth and a dummy
`organizationId`. A real route answers **400** (matched, bad request); a missing
route answers **404**.

| Resource | Status | Verdict |
| --- | --- | --- |
| organizations, users, collections, widgets, columns, cards, tags, comments, customfields, tasklists, tasks, webhooks, groups | **400** | ✅ exists |
| contacts, relations, expenses, schedulers, calendar, categoryfields, subtasks, attachments, lanes, activities, sessions, me | **404** | ❌ does **not** exist as a top-level REST resource |

> The user-suspected extras (contacts, relations, expenses, schedulers,
> calendar) are **not** part of the Favro REST API. Subtasks are modeled as
> *Tasks*; attachments are a sub-resource of cards/comments (`/…/attachment`);
> lanes are embedded in the Widget object.

---

*Compiled 2026-07-23. When the official SPA at <https://favro.com/developer/>
becomes statically retrievable, cross-check any `*(partial)*` note above against
it.*
