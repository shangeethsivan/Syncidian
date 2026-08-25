# Syncidian architecture

This document describes the **current MVP** as implemented in this repository. GitHub renders the Mermaid diagrams below — open this file on GitHub to visualize them.

The plugin is the client. The Go server is the coordination layer. A per-user vault on disk plus SQLite metadata is the working copy. GitHub is an **optional, per-user** durable source of truth — people sign in with GitHub on the public landing, then install the App on one repository. Operators use `/admin`. MCP is the AI bridge.

When you change this architecture, update the Mermaid diagrams in this file and in `README.md`. See [`AGENT.md`](../AGENT.md).

---

## 1. System overview

```mermaid
flowchart TB
  subgraph clients [Clients]
    Win["Obsidian plugin<br/>Windows"]
    Mac["Obsidian plugin<br/>macOS / Linux"]
    And["Obsidian plugin<br/>Android"]
    iOS["Obsidian plugin<br/>iOS"]
    Dash["Web dashboard<br/>browser"]
    AI["MCP clients<br/>Claude · Gemini · agents"]
  end

  subgraph server [Syncidian server — Go]
    API["HTTP API /api/v1"]
    WS["WebSocket hub<br/>/api/v1/ws"]
    Sync["Sync engine<br/>plan · push · pull"]
    Auth["Auth<br/>session cookie or Bearer token"]
    MCP["MCP JSON-RPC<br/>POST /mcp"]
    Git["gitx<br/>commit · push · pull"]
    Web["Embedded dashboard<br/>internal/web"]
  end

  subgraph data [Per-user data on the server]
    SQLite[("SQLite<br/>data/syncidian.db")]
    Vault[("Vault files<br/>data/vaults/{userId}")]
  end

  GH["Per-user private GitHub repo<br/>optional · after login"]

  Win -->|"HTTPS + WebSocket"| API
  Mac --> API
  And --> API
  iOS --> API
  Dash -->|"session cookie"| API
  AI -->|"Bearer token"| MCP

  API --> Auth
  API --> Sync
  API --> WS
  API --> Web
  Sync --> Vault
  Sync --> SQLite
  Auth --> SQLite
  MCP --> Vault
  MCP --> SQLite
  Sync --> Git
  Git --> Vault
  Git -->|"GitHub App installation token"| GH
```

---

## 2. Server packages

```mermaid
flowchart LR
  CMD["cmd/syncidian<br/>serve · user · token"] --> CFG["internal/config"]
  CMD --> SRV["internal/server"]
  SRV --> ST["internal/store<br/>SQLite + vault dirs"]
  SRV --> ENG["internal/syncengine<br/>PlanSync · ClassifyPush"]
  SRV --> GIT["internal/gitx"]
  SRV --> GHA["internal/githubapp"]
  SRV --> MCP["internal/mcp"]
  SRV --> WEB["internal/web<br/>embed index.html"]
```

`cmd/syncidian` boots the process. `internal/server` owns HTTP routes, auth, sync, GitHub, WebSocket, and MCP wiring. Persistence lives in `internal/store`. Sync decisions are pure functions in `internal/syncengine` so they can be unit-tested without I/O.

---

## 3. HTTP surface

```mermaid
flowchart TB
  subgraph public [Unauthenticated — public site]
    H["GET /health · /ready"]
    UI["GET /  → one-page landing"]
    AdminUI["GET /admin → admin sign-in / first setup"]
    S1["GET/POST /api/v1/setup"]
    L["POST /api/v1/auth/login · /auth/signup"]
    GHO["GET /api/v1/auth/github/start · /callback"]
    AppPub["GET /api/v1/github/app/setup · /callback<br/>GET/POST /api/v1/github/app/webhook · /urls"]
  end

  subgraph session [Dashboard session cookie — after login]
    Me["GET /api/v1/me · /stats"]
    Users["GET/POST /api/v1/users<br/>admin: public fields only"]
    Tokens["GET/POST /api/v1/tokens"]
    Dev["devices · heartbeat"]
    GH["GET/POST /api/v1/github<br/>POST /api/v1/github/app/start"]
    AppAdm["POST /api/v1/github/app/register<br/>admin: instance App"]
    MCPCfg["GET/POST /api/v1/mcp"]
    Act["GET /api/v1/activity"]
  end

  subgraph plugin [Bearer sk_sync_ token]
    Reg["POST /api/v1/devices/register"]
    Plan["POST /api/v1/sync/plan"]
    Push["POST /api/v1/sync/push"]
    File["GET /api/v1/sync/file"]
    Man["GET /api/v1/sync/manifest"]
    Conf["conflicts list · get · resolve"]
    Sock["GET /api/v1/ws"]
    MCP["POST /mcp"]
  end

  public --> session
  session --- plugin
```

Both the dashboard cookie (`syncidian_session`) and the plugin/MCP Bearer token go through the same `authenticate()` path. Tokens are stored as SHA-256 hashes; the raw `sk_sync_…` value is shown once. `GET/POST /api/v1/github` is never public — unauthenticated callers receive 401. GitHub App **callback**, **setup**, and **webhook** URLs are public so GitHub can redirect and ping.

---

## 3b. App workflow

This is the user-facing path the dashboard implements. Keep it in sync with `internal/web/static/index.html`.

```mermaid
flowchart TD
  Open["GET /"] --> Land["Landing: what Syncidian is"]
  Land --> GH["Sign up / Log in / Connect with GitHub"]
  Land --> Email["Optional email signup"]
  Land --> AdminLink["GET /admin"]

  AdminLink --> Auth{"Authenticated admin?"}
  Auth -->|no, empty DB| Form["Create first admin"]
  Auth -->|no| Login["POST /api/v1/auth/login"]
  Form --> Cookie["HttpOnly cookie"]
  Login --> Cookie
  Cookie --> Manage["Users + instance GitHub App URLs"]

  GH --> OAuth["GET /api/v1/auth/github/callback"]
  OAuth --> User["Regular user session"]
  Email --> User
  User --> Own["Own vault, devices, tokens, activity"]
  Own --> Backup{"User wants backup?"}
  Backup -->|yes| One["Install GitHub App<br/>one repo for this user_id · main only"]
  Backup -->|no| SyncOnly["Device sync still works"]
```

---

## 4. Plugin startup and first sync

```mermaid
sequenceDiagram
  participant User
  participant Obsidian
  participant Plugin as Syncidian plugin
  participant API as Syncidian server
  participant DB as SQLite + vault

  User->>Obsidian: Open vault
  Obsidian->>Plugin: onload + layout ready
  Plugin->>API: POST /api/v1/devices/register
  API->>DB: UpsertDevice
  API-->>Plugin: device id

  Plugin->>Plugin: Hash every local file SHA-256
  Plugin->>API: POST /api/v1/sync/plan<br/>path, hash, base_hash
  API->>DB: ListFiles
  API-->>Plugin: Pull / Push / Conflicts

  loop Each path in Pull
    Plugin->>API: GET /api/v1/sync/file?path=
    API->>DB: Read vault bytes
    API-->>Plugin: base64 content
    Plugin->>Obsidian: create/modify/delete
  end

  opt Paths in Push
    Plugin->>API: POST /api/v1/sync/push
    API->>DB: Write files + metadata
    API-->>Plugin: accepted + conflicts
  end

  Plugin->>API: WebSocket /api/v1/ws
  Note over Plugin,API: Live file_changed and github_synced events
```

After the first plan/push, the plugin keeps a local `hashes` map (last-known server hash per path). That map is the `base_hash` used on later syncs.

---

## 5. Editing a note

```mermaid
sequenceDiagram
  participant User
  participant Vault as Obsidian vault
  participant Plugin as Plugin
  participant API as Server
  participant Hub as WebSocket hub
  participant Other as Other devices
  participant Git as gitx
  participant GH as GitHub

  User->>Vault: Edit note
  Vault->>Plugin: create / modify / delete (debounced 400ms)
  Plugin->>API: POST /api/v1/sync/push
  API->>API: ClassifyPush(client, server, base)
  alt accept
    API->>API: Write vault file + UpsertFile
    API->>Hub: Broadcast file_changed (skip sender)
    Hub->>Other: pullFile(path)
    API->>Git: CommitAll
    opt GitHub configured
      Git->>GH: Push
    end
  else conflict
    API->>API: CreateConflict both versions
    API-->>Plugin: conflicts[]
    Plugin->>User: Conflict modal
  end
```

The user never runs git. The plugin never holds GitHub credentials.

---

## 6. Sync plan decisions

`PlanSync` compares three hashes per path: the client's current file, the server's current file, and the client's last-known server hash (`base`).

```mermaid
flowchart TD
  Start["For each client path"] --> OnServer{"On server?"}
  OnServer -->|no| Push["Push"]
  OnServer -->|yes| Same{"client hash == server hash?"}
  Same -->|yes| Skip["No-op"]
  Same -->|no| BaseS{"base == server?"}
  BaseS -->|yes| Push
  BaseS -->|no| BaseC{"base == client?"}
  BaseC -->|yes| Pull["Pull"]
  BaseC -->|no| Conflict["Conflict"]

  Start2["Server paths the client never sent"] --> Pull
```

`ClassifyPush` is the write-side counterpart: accept, no-op, or raise a conflict before bytes are written.

---

## 7. Conflict resolution

The first version is human-in-the-loop. A small-LLM auto-resolver is on the roadmap, not in this MVP.

```mermaid
flowchart TD
  Detect["Same path changed on two devices<br/>before sync completed"] --> StoreC["Server stores both blobs<br/>conflicts table"]
  StoreC --> UI["Plugin ConflictModal<br/>or dashboard Conflicts"]
  UI --> KeepL["Keep local"]
  UI --> KeepR["Keep remote"]
  UI --> Merge["Paste merged text"]
  KeepL --> Resolve["POST /api/v1/conflicts/id/resolve"]
  KeepR --> Resolve
  Merge --> Resolve
  Resolve --> Write["Write chosen bytes to vault"]
  Write --> Broadcast["WebSocket file_changed"]
  Write --> Commit["Git commit + optional GitHub push"]
```

---

## 8. Authentication

```mermaid
flowchart LR
  subgraph site [Public site]
    Land["GET / landing"] --> GitHub["Sign in with GitHub"]
    Land --> Email["POST /api/v1/auth/signup"]
    GitHub --> Callback["GET /api/v1/auth/github/callback"]
    Callback --> Cookie["HttpOnly cookie<br/>syncidian_session"]
    Email --> Cookie
  end

  subgraph admin [Operators]
    AdminUI["GET /admin"] --> Setup["First-boot: create admin"]
    AdminUI --> Login["POST /api/v1/auth/login"]
    Setup --> Cookie
    Login --> Cookie
    Cookie --> Split{"Role"}
    Split -->|"admin"| UsersOnly["Users + GitHub App URLs<br/>no vault"]
    Split -->|"user"| VaultUI["Optional GitHub page<br/>one repo per user"]
  end

  subgraph pluginAuth [Plugin and MCP]
    Tokens["Dashboard Tokens<br/>or CLI: syncidian token create"] --> Raw["sk_sync_… shown once"]
    Raw --> Hash["SHA-256 stored in tokens"]
    Raw --> Header["Authorization: Bearer"]
  end

  Cookie --> Authed["authenticate()"]
  Header --> Authed
  Authed --> User["store.User"]
```

GitHub OAuth is how vault users sign in. Admins sign in at `/admin` with username and password. The instance GitHub App needs a callback URL, a setup URL, and a webhook URL so GitHub can redirect and ping.

---

## 9. MCP / AI

```mermaid
sequenceDiagram
  participant Client as MCP client
  participant API as POST /mcp
  participant MCP as internal/mcp
  participant Perms as mcp_permissions
  participant Vault as User vault

  Client->>API: Bearer token + JSON-RPC
  API->>MCP: Handle(user, body)
  MCP->>Perms: GetMCP(user)

  alt initialize
    MCP-->>Client: protocolVersion 2024-11-05
  else tools/list
    MCP-->>Client: search_notes · list_notes · read_note<br/>create_note · update_note (if allowed)
  else tools/call
    MCP->>Vault: Search / read / write
    MCP-->>Client: text result
  end
```

Default permissions are **search + read**. Create and modify are off until the dashboard enables them.

---

## 10. Data model

```mermaid
erDiagram
  users ||--o{ tokens : owns
  users ||--o{ sessions : owns
  users ||--o{ devices : owns
  users ||--o{ files : vault
  users ||--o{ conflicts : has
  users ||--o{ activity : logs
  users ||--o| github_config : backup
  users ||--o| mcp_permissions : grants

  users {
    text id PK
    text username
    text password_hash
    text email
    int github_id
    int is_admin
  }
  tokens {
    text id PK
    text user_id FK
    text token_hash
    text prefix
    text revoked_at
  }
  devices {
    text id PK
    text user_id FK
    text name
    text platform
    text last_seen_at
    int sync_count
  }
  files {
    text user_id PK
    text path PK
    text hash
    int deleted
  }
  conflicts {
    text id PK
    text path
    blob local_content
    blob remote_content
    text resolution
  }
  github_config {
    text user_id PK
    int app_id
    int installation_id
    text repo
    text branch
  }
  instance_github_app {
    int id PK
    int app_id
    text slug
    text client_id
  }
```

Vault bytes live on disk at `data/vaults/<userId>/`. SQLite (`data/syncidian.db`) stores metadata, auth, devices, conflicts, activity, GitHub config, the instance GitHub App, and MCP permissions.

---

## 11. Multi-user isolation

```mermaid
flowchart TB
  S["One Syncidian process"] --> Admin["Admin<br/>users only · no repo sync"]
  S --> A["User A"]
  S --> B["User B"]
  S --> C["User C"]

  A --> AD["Devices: Win · Mac · Android"]
  A --> AV["vaults/A + files where user_id=A"]
  A --> AG["GitHub repo A"]

  B --> BD["Devices: Win · iOS"]
  B --> BV["vaults/B"]
  B --> BG["GitHub repo B"]

  C --> CD["Devices: Mac"]
  C --> CV["vaults/C"]
```

A regular user only sees their own devices, tokens, vault, conflicts, activity, and one GitHub repository. Admins manage accounts and cannot call vault, token, device, activity, or GitHub APIs. The WebSocket hub broadcasts per `userID`.

Admins can create users and list `adminUserSummary` fields (`username`, `is_admin`, `created_at`). They do **not** receive another user's GitHub App credentials, repository, vault bytes, tokens, or activity. Admin login does not require `github_config`.

---

## 12. Deployment

```mermaid
flowchart LR
  subgraph host [Docker host / Railway / VPS]
    C["syncidian container<br/>listen :8080 or $PORT"]
    V[("Named volume /data<br/>SQLite + vaults")]
    C --> V
  end

  Browser["Browser dashboard"] --> C
  Plugin["Obsidian plugin"] --> C
  MCP["MCP client"] --> C
  C -->|"optional"| GH["GitHub"]
```

One container. No extra services for the basic install. Railway mounts a volume at `/data`; the Dockerfile does not declare `VOLUME` (that broke some builders).

---

## Related code

| Area | Path |
| --- | --- |
| Process entry | [`cmd/syncidian/main.go`](../cmd/syncidian/main.go) |
| HTTP routes + auth | [`internal/server/server.go`](../internal/server/server.go), [`internal/server/auth.go`](../internal/server/auth.go), [`internal/server/auth_github.go`](../internal/server/auth_github.go) |
| Plan / push / devices | [`internal/server/sync.go`](../internal/server/sync.go) |
| Sync decisions | [`internal/syncengine/plan.go`](../internal/syncengine/plan.go) |
| GitHub backup | [`internal/server/github.go`](../internal/server/github.go), [`internal/server/github_app.go`](../internal/server/github_app.go), [`internal/githubapp`](../internal/githubapp), [`internal/gitx/repo.go`](../internal/gitx/repo.go) |
| GitHub App self-host setup | [`docs/github-app.md`](github-app.md) |
| MCP tools | [`internal/mcp/mcp.go`](../internal/mcp/mcp.go) |
| Live updates | [`internal/server/ws.go`](../internal/server/ws.go) |
| Persistence | [`internal/store/store.go`](../internal/store/store.go) |
| Dashboard UI | [`internal/web/static/index.html`](../internal/web/static/index.html) |
| Obsidian plugin | [`plugin/main.ts`](../plugin/main.ts) |
| Keep diagrams current | [`AGENT.md`](../AGENT.md) |
