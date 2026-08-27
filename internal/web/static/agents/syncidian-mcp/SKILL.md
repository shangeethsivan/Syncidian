---
name: syncidian-mcp
description: Connect to a self-hosted Syncidian MCP server to search, read, and (when enabled) write Obsidian notes in the user's GitHub-backed vault. Use when the user wants vault search, backlinks, graph, or note edits through Syncidian.
---

# Syncidian MCP

Syncidian exposes JSON-RPC MCP at `/mcp` on the same origin as this skill. Authenticate with `Authorization: Bearer sk_sync_…`.

## Get a token

- Dashboard **Tokens** (shown once), or
- `POST /api/v1/mcp/login` with `{"username","password"}` (vault users only)

Read `/auth.md` on this origin.

## Client config (Cursor / Claude)

```json
{
  "mcpServers": {
    "syncidian": {
      "url": "https://YOUR_SYNCIDIAN_HOST/mcp",
      "headers": {
        "Authorization": "Bearer sk_sync_…"
      }
    }
  }
}
```

## Tools (default: search + read)

- `search_notes` — substring search of path and content
- `list_notes` — list markdown notes
- `list_files` — list any vault file (notes, images, PDFs); optional `prefix` / `ext`
- `find_related` — links and text overlap
- `suggest_note_path` — where to put a new note
- `read_note`, `get_outgoing_links`, `get_backlinks`, `get_graph`

Create/modify tools (`create_note`, `update_note`, `append_to_note`, `move_note`, `delete_note`, `bulk_move`, `bulk_add_links`) stay off until **MCP / AI** enables them. `move_note` and `bulk_move` relocate markdown and attachments. Writes require a connected GitHub repo. MCP does not save note bodies on the Syncidian server.

## Rules

- Paths are vault-relative (`Projects/Ideas.md`).
- Do not put tokens in query strings or logs.
- Admins cannot call `/mcp`.
