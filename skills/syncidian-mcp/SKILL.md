---
name: syncidian-mcp
description: Search, read, link, and write an Obsidian vault through a Syncidian MCP server. Use whenever the user mentions Syncidian, their Obsidian vault, wikilinks, backlinks, or asks an agent to find, file, or edit notes, including when Syncidian tools are already connected and the user did not say "skill".
---

# Syncidian MCP

Syncidian is the user's Obsidian vault, reached over MCP. Note bodies are not stored on the Syncidian server. Every list, search, read, and write uses that user's GitHub repository on `main`. A connected Obsidian app is notified after a write.

Call the Syncidian tools by name (`search_notes`, `read_note`, and the rest). If those tools are already in your tool list, skip connection and follow the workflows below.

## Connect only when the tools are missing

The endpoint is `POST {origin}/mcp` with `Authorization: Bearer sk_sync_…`. If this file was loaded from `/.well-known/agent-skills/` on a Syncidian host, that host is the origin. Otherwise use the host in the user's MCP config, or ask. Auth details are at `{origin}/auth.md`.

The token belongs to a vault user. Admins cannot call `/mcp`. Mint it on the dashboard **Tokens** page (shown once) or with `POST {origin}/api/v1/mcp/login` and `{"username","password"}`. Keep it out of query strings, notes, and logs.

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

## Permissions and GitHub

Search and read are on by default. Create and modify stay off until the user enables them under **Dashboard → MCP / AI**. `tools/list` hides tools whose toggle is off. `permission denied` means that toggle, not a broken call. Say which toggle, and stop.

Every note tool needs a connected GitHub repository. Until then the error is `connect a GitHub repository first; Syncidian does not store notes on the server`. Device sync can work without GitHub. MCP cannot.

| Permission | Tools |
| --- | --- |
| search | `search_notes`, `list_notes`, `list_files`, `find_related`, `suggest_note_path` |
| read | `read_note`, `get_outgoing_links`, `get_backlinks`, `get_graph` |
| create | `create_note` |
| modify | `update_note`, `add_backlink`, `move_note`, `delete_note`, `bulk_move`, `bulk_add_links` |
| create if the note is new, modify if it exists | `append_to_note` |

## Paths

Use vault-relative paths with forward slashes: `Projects/Ideas.md`. No leading slash and no `..`.

Note tools add `.md` when the path has no extension. `move_note` and `delete_note` also move images and other files; keep their real extension. A markdown move to a destination with no dot gets `.md`.

These paths are rejected: `.git`, `.trash`, `.obsidian/workspace.json`, `.obsidian/workspace-mobile.json`, `.obsidian/plugins/syncidian/`.

For a folder filter, include the trailing slash (`Projects/`). A prefix without it also matches paths that merely start with those letters.

## Look something up

Search is a case-insensitive substring of the path and the body, capped at 50 hits. It is not semantic. A distinctive word beats a full sentence. Results are JSON `{path, snippet}` or the text `No matches.`

`find_related` ranks by wikilinks plus word overlap (default 15, max 50). Pass a note path when you want that note's neighborhood.

1. `search_notes` or `find_related` first. List the whole vault only when the user asked for a map, or search found nothing and a folder prefix will help.
2. `read_note` only the hits you will use. `read_note` is for UTF-8 notes, not images or PDFs.
3. `get_backlinks` for "what links here". `get_outgoing_links` for "what does this cite" (`target`, resolved `path`, `exists`).
4. `get_graph` with a folder `prefix` when the question is about one area. Default `json` includes nodes, edges, and a Mermaid diagram. `format: mermaid` returns only the diagram.
5. `list_notes` is markdown. `list_files` is every file; filter with `prefix` and `ext` (`.png` or `png`).

Cite the vault path in the answer.

## Write

Read the note in this turn before you change it. `update_note` replaces the entire file. `create_note` fails if the path already exists. `update_note` fails if it does not.

New note:

1. `suggest_note_path` with `topic` and optional `kind` (`idea`, `project`, `meeting`, `note`). Kind defaults to `idea`.
2. Follow the tool hint. A high score in `existing_matches` means append to that note. Otherwise `create_note` at `suggested_new`.
3. Link it. `add_backlink` appends a `[[wikilink]]` from `from` to `to` and skips a duplicate. Same-folder links use the note name; other links use the path without `.md`. Optional `alias`.

Small addition: `append_to_note`. A `heading` that does not start with `#` is stored as `## heading`. If that heading line already exists, the text is inserted under it. Otherwise the heading and text are appended. This creates the file when create is allowed and the path is missing.

`move_note` refuses when the destination exists. `bulk_move` moves every file under `from_prefix`, or the `paths` you pass, into `to_prefix`. `bulk_add_links` adds one wikilink target to many notes. List real paths before a bulk call.

`delete_note` removes the file from GitHub `main`. Delete only after the user confirms that exact path.

A successful write returns text such as `Wrote …`, `Moved …`, or `Deleted …`. If the tool errors, the vault did not change.

## Examples

**Answer from the vault.** "What did I decide about the railway volume?"

1. `search_notes` with query `railway volume`.
2. `read_note` on the best path.
3. Answer, and name that path.

**File an idea.** "Save this: agents should search before listing the vault."

1. `suggest_note_path` with that topic and `kind` `idea`.
2. High-score match: `append_to_note` on that path.
3. Otherwise `create_note` at `suggested_new`, then `add_backlink` from a related note you already read.

**Edit one section.** "Add a line under Decisions in Projects/Syncidian.md."

1. `read_note` `Projects/Syncidian.md`.
2. `append_to_note` with `heading` `Decisions` and the new line.
3. Use `update_note` only when the user wants the whole note replaced, and send the full body you just read.
