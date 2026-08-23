# Syncidian

> **Sync your knowledge. Back it up. Give AI access to it — on your terms.**

Syncidian is an **open-source, self-hostable sync and AI bridge for Obsidian**.

It brings three things together:

* 🔄 **Sync** — Synchronize your Obsidian vault across devices.
* 🐙 **Backup** — Use GitHub as a private, versioned backup.
* 🤖 **AI** — Connect your knowledge base to AI tools through a built-in MCP server.

The Syncidian relay is designed to be **stateless**: it facilitates encrypted communication between devices without becoming a permanent storage location for your vault.

Self-host Syncidian completely free, or use the upcoming managed Syncidian service starting at **$1/month**.

---

## ✨ Why Syncidian?

Obsidian is a powerful local-first knowledge base. Syncidian extends that philosophy.

Instead of putting your vault into another proprietary sync platform, Syncidian lets you use infrastructure you control and technologies you already trust.

### With Syncidian, you can:

* 🔐 Sync your vault through an encrypted channel
* 🏠 Self-host the sync infrastructure
* 🚫 Keep the sync relay stateless
* 🐙 Back up your vault to a private GitHub repository
* 📚 Keep Git-based version history
* 🤖 Connect your vault to AI through MCP
* 🔎 Let AI search and understand your knowledge
* 📝 Allow controlled AI actions on your vault
* 💰 Self-host for free
* 🌐 Use the upcoming hosted service for **$1/month**

---

# 🏗️ Architecture

Syncidian separates **synchronization, storage, and AI access**.

```text
                         AI TOOLS
                ┌─────────────────────┐
                │ Claude · Gemini ·    │
                │ ChatGPT · Agents ·   │
                │ MCP-compatible tools │
                └──────────┬──────────┘
                           │
                          MCP
                           │
                           ▼
                 ┌───────────────────┐
                 │     Syncidian     │
                 │                   │
                 │    MCP Server     │
                 │         │         │
                 │    Sync Engine    │
                 └─────────┬─────────┘
                           │
                           │
             ┌─────────────┴─────────────┐
             │                           │
             ▼                           ▼
      ┌─────────────┐             ┌──────────────┐
      │   Obsidian  │             │    GitHub    │
      │   Devices   │             │ Private Repo │
      └─────────────┘             └──────────────┘
             │
             │ Encrypted Sync
             ▼
      ┌─────────────────┐
      │ Syncidian Relay │
      │ Stateless Sync  │
      └─────────────────┘
```

---

# 🔄 Sync

Syncidian provides a synchronization layer between your Obsidian devices.

```text
Obsidian Device A
       │
       │ Encrypted
       ▼
┌──────────────────┐
│ Syncidian Relay  │
│ Stateless        │
└────────┬─────────┘
         │
         │ Encrypted
         ▼
Obsidian Device B
```

The relay exists to **move changes between devices**.

It is not intended to be your permanent vault storage.

### Goals

* Multi-device synchronization
* Reliable change propagation
* Offline device support
* Conflict detection
* Conflict resolution
* Encrypted communication
* Minimal server-side persistence

---

# 🐙 GitHub Backup

Syncidian uses Git as the durable storage and versioning layer.

You can connect Syncidian to a **private GitHub repository**.

```text
Obsidian Vault
      │
      ▼
  Syncidian
      │
      ▼
 Private GitHub Repository
      │
      ├── Version history
      ├── Change tracking
      ├── Recovery
      └── Backup
```

This means your vault benefits from Git's existing capabilities:

* Version history
* Change tracking
* Rollbacks
* Recovery
* Branching
* Diffing

Your GitHub repository remains under your control.

---

# 🤖 AI + MCP

Syncidian isn't only a sync client.

It also includes a built-in **Model Context Protocol (MCP) server**.

This allows compatible AI tools and agents to interact with your Obsidian knowledge base.

```text
┌───────────────────────────┐
│         AI TOOLS          │
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
│      ┌─────────────┐      │
│      │ MCP Server  │      │
│      └──────┬──────┘      │
│             │             │
│      ┌──────▼──────┐      │
│      │ Obsidian    │      │
│      │ Vault       │      │
│      └─────────────┘      │
└───────────────────────────┘
```

## What can AI do?

The MCP implementation is intended to provide controlled access to your knowledge base.

Potential capabilities include:

* 🔎 Search your vault
* 📖 Read notes
* 📝 Create notes
* ✏️ Update notes
* 🗂️ Navigate your knowledge graph
* 🔗 Find related information
* 🧠 Use your existing knowledge as AI context
* ⚙️ Perform controlled actions

The available tools and permissions will evolve as Syncidian develops.

---

# 🧠 Your Knowledge, Connected to AI

Your Obsidian vault can become much more than a folder of Markdown files.

It can contain:

* Ideas
* Projects
* Documentation
* Research
* Meeting notes
* Personal knowledge
* Technical notes
* Long-term context

Syncidian aims to make that knowledge available to the AI tools you already use.

Instead of moving your entire knowledge base into an AI platform, Syncidian provides a bridge between your **local knowledge and AI agents**.

> **Your knowledge stays yours. AI comes to your knowledge.**

---

# 🔐 Security & Privacy

Syncidian is being designed around a few core principles:

### Encrypted communication

Communication between devices and the Syncidian relay is intended to happen over an encrypted channel.

### Stateless relay

The relay is designed to facilitate synchronization without permanently storing your vault.

### Private Git repository

GitHub can be configured with a private repository for durable backup.

### Self-hosting

You can run the entire Syncidian infrastructure yourself.

### Least-privilege AI access

MCP access should be independently controlled from synchronization.

Future security controls will include:

* AI client authentication
* Read-only access
* Read/write permissions
* Tool-level permissions
* Vault-level permissions
* Local-only MCP access
* Authentication and authorization

> **Security is still under active development. Do not assume Syncidian is production-ready for highly sensitive data until the security model has been independently reviewed.**

---

# 🏠 Self-Hosted

Syncidian is designed to be completely self-hostable.

```text
             Your Infrastructure

┌───────────────┐
│ Your Devices  │
└───────┬───────┘
        │
        ▼
┌───────────────────┐
│ Your Syncidian    │
│ Server            │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Your Private      │
│ GitHub Repository │
└───────────────────┘
```

You can deploy it on:

* VPS
* Home server
* Raspberry Pi
* Docker
* Docker Compose
* Cloud infrastructure
* Your own hardware

### Cost

**Syncidian self-hosted: Free**

You only pay for the infrastructure you choose to run it on.

---

# 🌐 Hosted Syncidian

Don't want to manage a server?

A managed Syncidian service is **coming soon**.

### Planned launch pricing

**Starting at $1/month**

The hosted service will provide:

* Managed Syncidian infrastructure
* Server maintenance
* Availability monitoring
* Easier device setup
* No server administration

The underlying Syncidian project will remain open source, and self-hosting will remain available.

---

# ⚡ Quick Start

> 🚧 **Syncidian is currently under active development.**

The intended setup will look like this:

### 1. Create a private GitHub repository

Create a private repository for your Obsidian vault.

### 2. Deploy Syncidian

Run the Syncidian server.

```bash
docker run ...
```

Docker and Docker Compose deployment instructions will be provided as the project reaches its first usable release.

### 3. Configure GitHub

Connect Syncidian to your private repository.

### 4. Connect your devices

Configure the Syncidian client on your Obsidian devices.

### 5. Connect your AI tools

Configure an MCP-compatible AI client to connect to the Syncidian MCP server.

### 6. Start syncing

Your devices synchronize through Syncidian while GitHub maintains your durable versioned backup.

---

# 🧩 Features

### Sync

* [ ] Encrypted device synchronization
* [ ] Multi-device support
* [ ] Offline synchronization
* [ ] Conflict detection
* [ ] Conflict resolution
* [ ] Stateless relay

### Git

* [ ] GitHub authentication
* [ ] Private repository support
* [ ] Automatic commits
* [ ] Version history
* [ ] Restore from Git
* [ ] Change tracking

### MCP / AI

* [ ] Built-in MCP server
* [ ] Vault search
* [ ] Note reading
* [ ] Note creation
* [ ] Note editing
* [ ] Related-note discovery
* [ ] Read-only permissions
* [ ] Read/write permissions
* [ ] MCP authentication

### Deployment

* [ ] Docker
* [ ] Docker Compose
* [ ] VPS deployment
* [ ] Home server deployment
* [ ] Health checks
* [ ] Configuration tooling

### Hosted

* [ ] Managed Syncidian
* [ ] Device management
* [ ] Usage dashboard
* [ ] Monitoring
* [ ] Billing
* [ ] Launch pricing from $1/month

---

# 🗺️ Roadmap

## Phase 1 — Sync Engine

* [ ] Define synchronization protocol
* [ ] Build relay
* [ ] Implement encrypted communication
* [ ] Device registration
* [ ] Basic synchronization

## Phase 2 — Git Backup

* [ ] Git integration
* [ ] GitHub authentication
* [ ] Private repository support
* [ ] Automatic commits
* [ ] Restore functionality
* [ ] Conflict handling

## Phase 3 — MCP

* [ ] MCP server
* [ ] Vault search
* [ ] Read tools
* [ ] Write tools
* [ ] Permissions
* [ ] Authentication

## Phase 4 — Self-hosting

* [ ] Docker image
* [ ] Docker Compose
* [ ] Configuration documentation
* [ ] Deployment guides
* [ ] Health monitoring

## Phase 5 — Hosted Syncidian

* [ ] Managed infrastructure
* [ ] User accounts
* [ ] Device management
* [ ] Monitoring
* [ ] Billing
* [ ] Launch at $1/month

---

# 🤔 Syncidian vs Obsidian Sync

|                     | Syncidian         | Obsidian Sync |
| ------------------- | ----------------- | ------------- |
| Self-hosted         | ✅                 | ❌             |
| Open source         | ✅                 | ❌             |
| GitHub backup       | ✅                 | ❌             |
| Git version history | ✅                 | ❌             |
| Own infrastructure  | ✅                 | ❌             |
| Built-in MCP        | ✅                 | ❌             |
| Managed hosting     | 🚧 Coming soon    | ✅             |
| Self-hosted cost    | **Free**          | —             |
| Hosted Syncidian    | **From $1/month** | —             |

Syncidian isn't intended to reproduce every feature of Obsidian Sync.

It takes a different approach:

> **Sync your vault through a lightweight relay, keep durable storage in Git, and make your knowledge accessible to AI through MCP.**

---

# 💡 Philosophy

Syncidian is built around three principles:

### 1. Sync should be a service

A sync server doesn't necessarily need to permanently own your data.

### 2. Storage should be yours

Git provides a mature, transparent, versioned way to store and recover your files.

### 3. AI should come to your knowledge

Your knowledge base shouldn't need to be copied into every AI platform.

Syncidian aims to provide the bridge.

```text
             SYNCIDIAN

       ┌─────────────────────┐
       │                     │
       │    🔄 SYNC          │
       │    Devices          │
       │                     │
       ├─────────────────────┤
       │                     │
       │    🐙 BACKUP        │
       │    Git + GitHub     │
       │                     │
       ├─────────────────────┤
       │                     │
       │    🤖 AI            │
       │    MCP + Agents     │
       │                     │
       └─────────────────────┘
```

> **Sync your knowledge. Back it up. Give AI access to it — on your terms.**

---

# 🚧 Project Status

**Syncidian is currently under active development.**

The initial goal is to make self-hosted deployment extremely simple, ideally allowing users to get started with a single Docker command.

Expect breaking changes while the architecture and protocol evolve.

---

# 🤝 Contributing

Syncidian is open source and contributions are welcome.

You can contribute through:

* Code
* Architecture discussions
* Bug reports
* Security reviews
* Documentation
* Testing
* Feature proposals

Contribution guidelines will be added as the project matures.

---

# 📄 License

Syncidian is licensed under the **MIT License**.

You are free to use, modify, distribute, and self-host Syncidian, including for commercial purposes, subject to the terms of the MIT License.

See [`LICENSE`](LICENSE) for the full license text.

---

<p align="center">
  <strong>Syncidian</strong><br>
  Sync your knowledge. Keep your data. Connect your AI.
</p>
