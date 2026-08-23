# Syncidian

> **Sync your knowledge. Back it up. Connect your AI. Own your data.**

Syncidian is an open-source, self-hostable **Obsidian synchronization and AI bridge**.

It runs as an Obsidian plugin on your devices and connects to a lightweight Syncidian server that coordinates synchronization between them.

The server manages GitHub integration centrally, while GitHub acts as the durable, versioned **source of truth** for your vault.

Syncidian also includes a built-in **MCP server**, allowing compatible AI tools and agents to securely interact with your Obsidian knowledge base.

The server is designed to be written in **Go** and deployed with a simple **Dockerfile**, making self-hosting as easy as:

```bash
docker build -t syncidian .
docker run -d \
  --name syncidian \
  -p 8080:8080 \
  syncidian
```

**Self-host Syncidian for free, or use the upcoming hosted service starting at $1/month.**

---

# 🚀 Quick start (Mac, Linux, or Windows with Docker)

This repository is ready to run. Clone it, start the server, sideload the Obsidian plugin, and sync.

## 1. Start the server

**With Docker (recommended):**

```bash
git clone https://github.com/shangeethsivan/Syncidian.git
cd Syncidian
docker compose up --build -d
```

Open [http://localhost:8080](http://localhost:8080), create the first admin user, then go to **Tokens** and create an access token (`sk_sync_…`). Copy it once — it is not shown again.

**Without Docker (Go 1.22+):**

```bash
go run ./cmd/syncidian serve
```

Data is stored in `./data` by default (`SYNCIDIAN_DATA` to change it). Docker Compose persists it at `/data` via a named volume (not a Dockerfile `VOLUME`, which Railway and some builders reject).

### Deploy on Railway

1. New project → deploy this GitHub repo. Railway builds the `Dockerfile`.
2. **Settings → Volumes → Add volume**, mount path **`/data`**.
3. Generate a public domain. The server listens on Railway’s `PORT` and uses `RAILWAY_PUBLIC_DOMAIN` for the dashboard URL unless you set `SYNCIDIAN_PUBLIC_URL`.
4. Optional variables: `SYNCIDIAN_BOOTSTRAP_USER`, `SYNCIDIAN_BOOTSTRAP_PASSWORD`.
5. Open the public URL, create the admin user, then point the plugin at that URL.

Health check: `GET /health`.

## 2. Install the Obsidian plugin (manual — required)

The plugin is **not** in the Obsidian Community Plugin store yet. Every vault/machine needs a one-time sideload. There is no extra server-side plugin deployment.

```bash
# from this repository
chmod +x scripts/install-plugin.sh
./scripts/install-plugin.sh "/path/to/YourVault"
```

On a Mac this is often something like:

```bash
./scripts/install-plugin.sh "$HOME/Documents/Obsidian/MyVault"
```

Or copy these files by hand into `{Vault}/.obsidian/plugins/syncidian/`:

* `plugin/manifest.json`
* `plugin/main.js`
* `plugin/styles.css`

Then in Obsidian:

1. Settings → Community plugins → turn **Restricted mode off** (allow community plugins).
2. Reload plugins / restart Obsidian.
3. Enable **Syncidian**.
4. Settings → Syncidian:
   * Server URL: `http://localhost:8080` (or your deployed URL)
   * Access token: the `sk_sync_…` value from the dashboard
   * Device name: e.g. `MacBook Pro`
5. Click **Connect**.

Repeat the plugin copy + token on each device (Windows, Mac, Android, iOS). Create one token per person; the same user can register many devices.

## 3. Optional: GitHub backup

In the dashboard → **GitHub**, paste a GitHub personal access token with `repo` scope and `owner/name`. The plugin never needs GitHub credentials. Until GitHub is connected, devices still sync through the server.

## 4. Optional: MCP / AI

Dashboard → **MCP / AI** sets tool permissions (search/read on by default). Point an MCP client at:

```text
POST http://localhost:8080/mcp
Authorization: Bearer sk_sync_…
```

---

# 📦 Publishing the Obsidian plugin

**You do not need to publish anything to start using Syncidian.** Sideloading from this repo is enough.

If you later want it in Obsidian’s Community Plugin browser, that is a **manual** process (Obsidian does not auto-publish from this repository):

1. Keep `plugin/manifest.json`, `plugin/main.js`, and `plugin/styles.css` in git (already done).
2. Create a GitHub Release whose tag matches `manifest.json` `version` (for example `0.1.0`) and attach those three files, or a `syncidian.zip` containing them.
3. Open a pull request against [obsidianmd/obsidian-releases](https://github.com/obsidianmd/obsidian-releases) adding Syncidian to `community-plugins.json`.
4. Wait for Obsidian’s review. They may ask for changes. This can take days to weeks.
5. After approval, users can install from Community plugins → Browse → “Syncidian” instead of copying files.

Until that review lands, tell testers to use `scripts/install-plugin.sh` or the [BRAT](https://github.com/TfTHacker/obsidian42-brat) plugin pointed at this GitHub repo.

Rebuilding the plugin after TypeScript changes:

```bash
cd plugin && npm install && npm run build
```

`plugin/main.js` is the compiled artifact Obsidian loads. Commit it so people can install without Node.js.

---

# ✨ What is Syncidian?

Syncidian brings three capabilities together:

* 🔄 **Sync** — Synchronize your Obsidian vault across devices.
* 🐙 **Backup** — Use a private GitHub repository as the durable source of truth.
* 🤖 **AI** — Connect your knowledge base to AI tools through MCP.

The goal is simple:

> **Your Obsidian vault should belong to you, be backed up by you, and be accessible to the AI tools you choose.**

---

# 🧠 The Vision

Syncidian isn't just another file synchronization tool.

The long-term goal is to make synchronization almost invisible.

```text
                 ┌─────────────────────────┐
                 │       AI TOOLS          │
                 │                         │
                 │ Claude · Gemini · Agents│
                 │       MCP Clients       │
                 └────────────┬────────────┘
                              │
                             MCP
                              │
                              ▼
┌─────────────┐      ┌──────────────────────┐
│  Obsidian   │◄────►│      Syncidian       │
│  Windows    │      │       Server         │
└─────────────┘      │                      │
                     │  Sync + Git + MCP    │
┌─────────────┐      │  Auth + Dashboard    │
│  Obsidian   │◄────►│                      │
│   macOS     │      └──────────┬───────────┘
└─────────────┘                 │
                                │
┌─────────────┐                 │
│  Obsidian   │◄────────────────┤
│   Android   │                 │
└─────────────┘                 │
                                ▼
┌─────────────┐        ┌─────────────────────┐
│  Obsidian   │        │    Private GitHub   │
│    iOS      │        │      Repository     │
└─────────────┘        │  Source of Truth    │
                       └─────────────────────┘
```

Eventually, Syncidian should be able to detect a conflict, have a small LLM resolve it automatically, validate the result, commit it to GitHub, and propagate the resolution to every device.

> **Let AI handle the boring conflicts. Let humans handle the important ones.**

---

# 🏗️ Architecture

Syncidian separates the system into four major components:

```text
┌────────────────────────────────────────────────────┐
│                    SYNCIDIAN                       │
│                                                    │
│  ┌──────────────┐                                  │
│  │ Obsidian     │                                  │
│  │ Plugin       │                                  │
│  └──────┬───────┘                                  │
│         │                                           │
│         │ HTTPS / WebSocket                         │
│         ▼                                           │
│  ┌──────────────────────────────────────────────┐   │
│  │             Syncidian Server                │   │
│  │                                              │   │
│  │  Sync Engine                                │   │
│  │  Authentication                             │   │
│  │  Device Management                          │   │
│  │  Git Integration                            │   │
│  │  MCP Server                                 │   │
│  │  Conflict Resolution                        │   │
│  │  Web Dashboard                              │   │
│  └─────────────┬────────────────┬─────────────┘   │
│                │                │                  │
│                ▼                ▼                  │
│         ┌─────────────┐   ┌───────────────┐       │
│         │   GitHub    │   │ Small LLM     │       │
│         │             │   │ Conflict      │       │
│         │ Source of   │   │ Resolver      │       │
│         │ Truth       │   │               │       │
│         └─────────────┘   └───────────────┘       │
└────────────────────────────────────────────────────┘
```

---

# 🔌 Obsidian Plugin

Syncidian lives inside Obsidian as a plugin.

There is no separate sync application that the user needs to manually operate.

The plugin handles:

* Detecting vault changes
* Sending changes to the Syncidian server
* Fetching changes
* Registering the device
* Detecting conflicts
* Showing conflict resolution UI
* Connecting to MCP
* Reporting client status
* Reporting synchronization state

The user configures the plugin once.

After that, Syncidian works automatically in the background.

---

# 💻 Supported Platforms

Syncidian is designed to work wherever Obsidian plugins are supported.

### Desktop

* 🪟 Windows
* 🍎 macOS
* 🐧 Linux

### Mobile

* 🤖 Android
* 📱 iOS

The goal is to maintain one consistent synchronization experience across all supported platforms.

---

# ⚙️ Setup

Syncidian is designed around a **server-first configuration model**.

The Obsidian plugin should require as little configuration as possible.

## 1. Deploy Syncidian Server

Run Syncidian on your own infrastructure.

The simplest deployment is Docker Compose from this repository:

```bash
docker compose up --build -d
```

Equivalent one-container flow:

```bash
docker build -t syncidian .
docker run -d \
  --name syncidian \
  -p 8080:8080 \
  -v syncidian-data:/data \
  syncidian
```

A future release should also provide a pre-built Docker image so users can simply run:

```bash
docker run -d \
  --name syncidian \
  -p 8080:8080 \
  ghcr.io/<owner>/syncidian:latest
```

---

# 🐳 One-Command Deployment

The goal is for Syncidian to be deployable with minimal infrastructure knowledge.

Eventually:

```bash
docker run -d \
  --name syncidian \
  -p 8080:8080 \
  -v syncidian-data:/data \
  ghcr.io/<owner>/syncidian:latest
```

The container should provide the complete Syncidian server:

* API
* Sync engine
* Authentication
* GitHub integration
* MCP server
* Web dashboard
* Device management
* Conflict resolution
* Health checks

No separate services should be required for the basic deployment.

---

# 🐙 Configure GitHub

GitHub configuration happens **on the Syncidian server**.

The Obsidian plugin does not need direct GitHub credentials.

The server administrator configures:

* GitHub authentication
* Private repository
* Repository mapping
* Git configuration
* Backup settings

Example:

```text
Syncidian Server
       │
       ▼
GitHub Authentication
       │
       ▼
Private Repository
```

This keeps GitHub credentials and Git operations centralized.

---

# 👤 Create a User

The Syncidian server can support multiple users.

Each user can have:

* Multiple devices
* Access tokens
* A configured vault
* A GitHub repository
* Synchronization history

Example:

```text
Syncidian Server

User A
 ├── Windows
 ├── macOS
 └── Android

User B
 ├── Windows
 └── iOS

User C
 └── macOS
```

Users are isolated from one another.

---

# 🔑 Access Tokens

Once a user is created, the server generates an access token.

```text
sk_sync_********************************
```

The token is used by the Obsidian plugin to authenticate with the Syncidian server.

The token should provide access only to the user's configured resources.

Future authentication options may include:

* Personal access tokens
* Device-specific tokens
* Token rotation
* Token revocation
* OAuth
* Passkeys
* SSO

---

# 🔌 Configure the Obsidian Plugin

Install Syncidian inside Obsidian.

The plugin configuration should remain intentionally small:

```text
Syncidian
────────────────────────────────

Server URL
https://sync.example.com

Access Token
••••••••••••••••••

Device Name
MacBook Pro

[ Connect ]

Status: ● Connected
```

The client does **not** require:

* GitHub credentials
* GitHub repository configuration
* Git credentials
* Manual Git commands

---

# 🚀 Activate

Once the plugin authenticates successfully:

1. Register the device.
2. Connect to the Syncidian server.
3. Identify the user's sync group.
4. Check the current source of truth.
5. Determine local changes.
6. Fetch required changes.
7. Start monitoring the vault.
8. Begin synchronization.

Repeat the process on other devices.

---

# 🔄 Synchronization

The Syncidian server is the **coordination layer**.

GitHub is the **primary source of truth**.

```text
                 GitHub
              Source of Truth
                    ▲
                    │
                    │ Git
                    │
            ┌───────┴────────┐
            │   Syncidian    │
            │     Server     │
            └───────┬────────┘
                    │
          ┌─────────┼─────────┐
          │         │         │
          ▼         ▼         ▼
       Windows     macOS    Android
       Obsidian   Obsidian  Obsidian
```

The server coordinates changes between clients and synchronizes the durable state with GitHub.

---

# 📝 Editing a Note

When a user edits a note:

```text
Edit note
   │
   ▼
Obsidian Plugin
   │
   │ Detect change
   ▼
Syncidian Server
   │
   ├───────────────► Other Devices
   │
   ▼
GitHub
   │
   ▼
Source of Truth
```

The user does not need to manually:

* Commit
* Push
* Pull
* Refresh
* Trigger sync

The plugin handles this automatically.

---

# 🚀 Opening Obsidian

Whenever Obsidian starts:

```text
Open Obsidian
      │
      ▼
Syncidian Plugin Starts
      │
      ▼
Authenticate
      │
      ▼
Connect to Syncidian
      │
      ▼
Check GitHub
      │
      ▼
Compare Local State
      │
      ▼
Fetch / Merge / Sync
      │
      ▼
Vault Ready
```

The goal is:

> **Open Obsidian and start working.**

Synchronization should happen automatically in the background.

---

# ⚔️ Conflict Resolution

Conflicts happen when the same file is modified on multiple devices before synchronization completes.

Syncidian should never silently overwrite user data.

The first version can provide an Obsidian-native conflict UI.

```text
┌─────────────────────────────────────────┐
│ ⚠️ Sync Conflict                        │
│                                         │
│ "Project Ideas.md" was modified on     │
│ another device.                         │
│                                         │
│ Local version     Remote version        │
│ ─────────────     ─────────────         │
│ Modified 10:32    Modified 10:34        │
│                                         │
│ [ Keep Local ] [ Keep Remote ]          │
│                                         │
│             [ Merge ]                   │
└─────────────────────────────────────────┘
```

Users can:

* Keep Local
* Keep Remote
* Review differences
* Merge manually
* Resolve later

---

# 🧠 AI-Assisted Conflict Resolution

A major future goal is to make merge conflicts almost invisible.

Instead of requiring the user to manually resolve every conflict, Syncidian can use a **small LLM** running alongside the Syncidian server.

```text
Device A
   │
   │ Change
   ▼
Syncidian Server
   │
   │ Conflict detected
   ▼
┌──────────────────────┐
│   Conflict Resolver  │
│                      │
│      Small LLM       │
└──────────┬───────────┘
           │
           ▼
┌────────────────────────┐
│ Local Version           │
│ Remote Version          │
│ Git History             │
└───────────┬────────────┘
            │
            ▼
      Resolved File
            │
            ▼
       Validation
            │
            ▼
      GitHub Commit
            │
            ▼
       Other Devices
```

## How it works

When a conflict occurs:

1. Syncidian detects the conflict.
2. The server collects the relevant versions.
3. The conflict resolver sends the required context to a small LLM.
4. The model proposes a merged version.
5. Syncidian validates the result.
6. If confidence is high enough, the server commits the result.
7. The resolved state is propagated to other devices.
8. If the conflict is ambiguous, the user is asked to resolve it manually.

---

# 👤 Human-in-the-Loop

AI should not blindly overwrite important information.

```text
                  Conflict
                     │
                     ▼
               Small LLM
                Resolver
                     │
             ┌───────┴───────┐
             │               │
        High confidence   Ambiguous
             │               │
             ▼               ▼
        Auto-resolve      User review
             │               │
             ▼               ▼
          GitHub         Obsidian UI
```

The goal is:

> **Let AI handle the boring conflicts. Let humans handle the important ones.**

---

# 🤖 MCP Server

Syncidian includes a built-in **Model Context Protocol (MCP) server**.

This provides a controlled interface between your Obsidian knowledge base and AI tools.

```text
┌───────────────────────────┐
│         AI Tools          │
│                           │
│ Claude · Gemini · Agents  │
│ Other MCP Clients         │
└─────────────┬─────────────┘
              │
              │ MCP
              ▼
┌───────────────────────────┐
│         Syncidian         │
│                           │
│       MCP Server          │
│            │              │
│            ▼              │
│      Obsidian Vault       │
└───────────────────────────┘
```

Potential capabilities:

* Search notes
* Read notes
* List notes
* Find related notes
* Create notes
* Update notes
* Navigate knowledge
* Provide context to AI agents

---

# 🧠 Your Knowledge, Connected to AI

An Obsidian vault can contain years of:

* Ideas
* Projects
* Research
* Documentation
* Meeting notes
* Technical knowledge
* Personal notes

Syncidian makes that knowledge available to AI through MCP.

Instead of permanently moving your knowledge into an AI platform, Syncidian provides a bridge between your local knowledge and the AI tools you choose.

> **Your knowledge stays yours. AI comes to your knowledge.**

---

# 🔐 MCP Permissions

AI access should be independently controllable.

Example:

```text
MCP Permissions

☑ Search notes
☑ Read notes
☐ Create notes
☐ Modify notes
```

Future controls can include:

* Read-only access
* Read/write access
* Tool-level permissions
* Vault-level permissions
* Client-level permissions
* AI client authentication
* Token revocation

The default configuration should follow **least privilege**.

---

# 📊 Web Dashboard

Syncidian includes a lightweight web dashboard.

The dashboard answers:

> **What is happening with my devices?**

Example:

```text
┌─────────────────────────────────────────────┐
│ Syncidian                                   │
│                                             │
│ DEVICES                                     │
│                                             │
│ ● MacBook Pro       macOS      Active       │
│ ● Pixel 10          Android    Active       │
│ ● Windows Desktop   Windows    Active       │
│ ○ iPhone            iOS        Offline      │
│                                             │
├─────────────────────────────────────────────┤
│                                             │
│ SYNC ACTIVITY                               │
│                                             │
│ Total Syncs                 1,248            │
│ Files Synced                 6,482           │
│ Last Sync                    2 min ago       │
│ Active Clients               3               │
│                                             │
└─────────────────────────────────────────────┘
```

### Dashboard capabilities

The dashboard should provide:

* Active clients
* Offline clients
* Device list
* Platform
* Last seen
* Last successful sync
* Number of syncs
* Files synchronized
* Recent sync activity
* Server health

The dashboard is for monitoring and management, not editing the vault.

---

# 👥 Multi-User Server

A single Syncidian server can support multiple users.

```text
                    Syncidian Server
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
      User A             User B             User C
        │                  │                  │
    ┌───┼───┐          ┌───┼───┐          ┌───┼───┐
    │   │   │          │   │   │          │   │   │
   Win Mac Android    Win Mac iOS        Mac Linux Android
```

Each user is isolated.

A user can only see their own:

* Devices
* Clients
* Sync activity
* Tokens
* Repository configuration
* Vault synchronization

---

# 📱 Device Management

A user can connect multiple Obsidian installations.

Example:

```text
My Devices

● MacBook Pro       macOS       Active
● Windows Desktop  Windows     Active
● Pixel             Android     Active
○ iPhone            iOS         Offline
```

The server tracks:

* Device name
* Platform
* Plugin version
* Connection status
* Last active time
* Last successful sync
* Sync count

---

# 🔑 Authentication

Syncidian uses access tokens to authenticate Obsidian clients.

```text
Server URL:
https://sync.example.com

Access Token:
sk_sync_********************************
```

Tokens should be:

* Revocable
* Scoped
* Rotatable
* Associated with users/devices

Future authentication options:

* Personal access tokens
* Device-specific tokens
* OAuth
* Passkeys
* SSO

---

# 🔐 Security & Privacy

Syncidian is designed around several principles.

### Encrypted communication

Client-to-server communication should use encrypted channels.

### GitHub credentials stay on the server

Obsidian clients never need direct GitHub credentials.

### User isolation

Multiple users can safely share a Syncidian server.

### Least privilege

Clients and AI tools should only receive required access.

### User-controlled infrastructure

Self-hosted users control their Syncidian server.

### Git-backed durability

GitHub provides durable versioned storage.

### Minimal relay persistence

The synchronization layer should avoid becoming a permanent storage location for vault data.

> **Security is still under active development. Do not use early builds for highly sensitive or critical vaults until the implementation and security model have been reviewed.**

---

# 🛠️ Technology

The initial architecture is intentionally simple.

### Server

**Go**

Go is a good fit for:

* Lightweight deployment
* Excellent concurrency
* Small container images
* Simple networking
* Easy cross-platform builds
* Long-running services

### Client

**Obsidian Plugin**

The client integrates directly with the Obsidian vault and lifecycle.

### Storage / Source of Truth

**Git + GitHub**

Private repositories provide durable, versioned vault storage.

### AI

**MCP + Small LLM**

MCP provides the interface to AI tools.

A small LLM can eventually provide automated conflict resolution.

### Deployment

**Docker**

The server should ideally be deployed as a single container.

---

# 🐳 Docker

The repository should contain a simple Dockerfile.

Example:

```dockerfile
FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -o syncidian ./cmd/syncidian

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/syncidian /app/syncidian

EXPOSE 8080

ENTRYPOINT ["/app/syncidian"]
```

The exact build configuration may evolve.

The important goal is:

> **Clone → Build → Run.**

---

# 🏠 Self-Hosted

Syncidian is completely free to self-host.

A basic installation can provide the entire system:

```text
┌─────────────────────────┐
│     Syncidian Server    │
│                         │
│  Sync Engine            │
│  GitHub Integration     │
│  MCP Server             │
│  Web Dashboard          │
│  Authentication         │
│  Device Management      │
│  Conflict Resolver      │
└─────────────────────────┘
```

### Cost

**$0 for the Syncidian software.**

You only pay for your own infrastructure.

This could be:

* Existing VPS
* Home server
* Raspberry Pi
* Cloud VM
* Docker host
* Local infrastructure

---

# 🌐 Hosted Syncidian

Don't want to manage your own server?

A managed Syncidian service is **coming soon**.

### Planned pricing

> **Starting at $1/month**

The hosted service will provide:

* Managed infrastructure
* GitHub integration
* Web dashboard
* Monitoring
* Server maintenance
* Multi-device management
* Easier setup

The self-hosted version will remain free and open source.

---

# 🧩 Feature Roadmap

## 🔄 Sync Engine

* [x] Obsidian plugin
* [x] File change detection
* [ ] Encrypted communication
* [x] Device registration
* [x] Multi-device synchronization
* [ ] Offline synchronization
* [x] Conflict detection
* [x] Conflict resolution UI
* [x] Background synchronization

## 🐙 GitHub

* [x] Server-side GitHub configuration
* [x] GitHub authentication
* [x] Private repository support
* [x] User-to-repository mapping
* [x] Automatic commits
* [x] Pull latest changes
* [ ] Restore from history
* [x] Git conflict handling

## 👥 Multi-User

* [x] User accounts
* [x] User isolation
* [x] Multiple devices per user
* [x] Access tokens
* [x] Device management
* [ ] Sync groups

## 📊 Dashboard

* [x] Web dashboard
* [x] Active clients
* [x] Offline clients
* [x] Last seen
* [x] Last successful sync
* [x] Sync count
* [x] Files synchronized
* [x] Recent activity
* [x] Server health

## 🤖 MCP / AI

* [x] Built-in MCP server
* [x] Vault search
* [x] Read notes
* [x] Create notes
* [x] Update notes
* [ ] Related-note discovery
* [x] Permission management
* [x] MCP authentication

## 🧠 AI Conflict Resolution

* [ ] Conflict extraction
* [ ] Small LLM integration
* [ ] Merge proposal generation
* [ ] Confidence scoring
* [ ] Result validation
* [ ] Automatic safe merges
* [ ] Human fallback
* [ ] Local model support
* [ ] Ollama-compatible inference
* [ ] Configurable AI provider

## 🔐 Authentication

* [x] Access tokens
* [x] Token revocation
* [ ] Token rotation
* [ ] Device tokens
* [ ] OAuth
* [ ] Passkeys

## 🚀 Deployment

* [x] Dockerfile
* [x] Single-container deployment
* [x] Docker Compose
* [ ] Pre-built container images
* [ ] GHCR publishing
* [x] VPS deployment guide
* [x] Home server deployment
* [x] Health checks
* [ ] Monitoring

## 🌐 Hosted Service

* [ ] Managed infrastructure
* [ ] User management
* [ ] Device management
* [ ] Monitoring
* [ ] Billing
* [ ] Launch at $1/month

---

# 🗺️ Development Phases

### Phase 1 — Obsidian Plugin + Core Sync

Build the fundamental synchronization experience.

```text
Obsidian
    ↕
Syncidian Server
    ↕
Obsidian
```

### Phase 2 — GitHub Source of Truth

Add Git-backed persistence, version history, recovery, and conflict handling.

### Phase 3 — Multi-User Server

Allow one Syncidian server to securely serve multiple users and devices.

### Phase 4 — Web Dashboard

Introduce device monitoring and synchronization statistics.

### Phase 5 — MCP / AI

Connect Obsidian knowledge to AI tools through MCP.

### Phase 6 — Hosted Syncidian

Provide a managed experience starting at $1/month.

### Phase 7 — AI Conflict Resolution

Use small LLMs to automatically resolve safe conflicts and fall back to human review when necessary.

---

# 🤔 Syncidian vs Obsidian Sync

|                                 | Syncidian         | Obsidian Sync |
| ------------------------------- | ----------------- | ------------- |
| Open source                     | ✅                 | ❌             |
| Self-hosted                     | ✅                 | ❌             |
| GitHub backup                   | ✅                 | ❌             |
| Git version history             | ✅                 | ❌             |
| Server-side Git configuration   | ✅                 | —             |
| Multi-user server               | ✅                 | —             |
| Device dashboard                | ✅                 | —             |
| Built-in MCP                    | ✅                 | ❌             |
| AI knowledge bridge             | ✅                 | —             |
| AI-assisted conflict resolution | 🚧 Planned        | —             |
| Hosted service                  | 🚧 Coming soon    | ✅             |
| Self-hosted cost                | **Free**          | —             |
| Hosted Syncidian                | **From $1/month** | —             |

Syncidian isn't trying to reproduce every feature of Obsidian Sync.

It takes a different approach:

> **The plugin is the client. Syncidian is the coordination layer. GitHub is the source of truth. MCP connects your knowledge to AI.**

---

# 💡 Philosophy

Syncidian is built around four principles.

### 1. Sync should be lightweight

The sync server coordinates devices instead of becoming another permanent copy of your vault.

### 2. Storage should be yours

Git provides transparent version history, recovery, and change tracking.

### 3. AI should come to your knowledge

Your knowledge base shouldn't need to be permanently copied into every AI platform.

### 4. Self-hosting should be accessible

Running your own Syncidian instance should be simple enough for anyone comfortable with Docker.

---

# 🚧 Project Status

**Syncidian is currently under active development.**

The architecture, synchronization protocol, plugin implementation, Git integration, dashboard, and MCP interface may change significantly before the first stable release.

The initial milestone is a reliable self-hosted synchronization experience with:

* Obsidian plugin
* Multi-device synchronization
* Server-side GitHub integration
* GitHub as the source of truth
* Conflict resolution
* Multi-user support
* Basic web dashboard
* Docker deployment

AI/MCP capabilities will be developed alongside the core synchronization system.

AI-assisted conflict resolution is a longer-term goal.

**Do not use early builds for critical or highly sensitive vaults.**

---

# 🤝 Contributing

Syncidian is open source and contributions are welcome.

You can contribute through:

* Code
* Architecture discussions
* Bug reports
* Documentation
* Testing
* Security reviews
* Feature proposals
* Platform-specific improvements

Contribution guidelines will be added as the project matures.

---

# 📄 License

Syncidian is licensed under the **MIT License**.

You are free to use, modify, distribute, and self-host Syncidian, including for commercial purposes, subject to the terms of the MIT License.

See [`LICENSE`](LICENSE) for the full license text.

---

<p align="center">
  <strong>Syncidian</strong><br>
  Sync your knowledge. Back it up. Connect your AI.
</p>
