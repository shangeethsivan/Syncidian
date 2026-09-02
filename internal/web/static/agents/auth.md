# Syncidian agent authentication

Agents do not use a browser cookie. Call protected APIs with an access token.

## Bearer token (required for MCP and vault sync)

Header:

```
Authorization: Bearer sk_sync_<hex>
```

Mint a token in one of these ways:

1. **Dashboard (humans):** sign in, open **Tokens**, create a token. The raw `sk_sync_…` value is shown once.
2. **Password exchange (agents / scripts):** `POST /api/v1/mcp/login` with JSON `{"username":"…","password":"…"}`. Vault users only. Admins are rejected. The response includes a one-time Bearer token. Use it on `POST /mcp`.
3. **Admin mint (operators):** an admin can create a one-time token for a vault user from **Users**. Admins cannot call `/mcp` themselves.

Tokens are stored hashed. They cannot connect or disconnect GitHub, change MCP permissions, or mint more tokens. They can `GET /api/v1/github/tree` and `POST /api/v1/github/sync` so the Obsidian vault matches GitHub after a remote restructure. Connect/disconnect and MCP settings need a dashboard session cookie (`syncidian_session`).

## MCP

```
POST {origin}/mcp
Authorization: Bearer sk_sync_…
Content-Type: application/json
```

JSON-RPC methods: `initialize`, `tools/list`, `tools/call`, `ping`.

Default tool permissions: search and read. Create and modify must be enabled on **MCP / AI**. Writes go to the user's GitHub repository, not the Syncidian disk copy of notes.

Discovery: [/.well-known/mcp/server-card.json](/.well-known/mcp/server-card.json)  
Protected resource metadata: [/.well-known/oauth-protected-resource](/.well-known/oauth-protected-resource)

## Human sign-in (not for agents)

People sign in with GitHub (`GET /api/v1/auth/github/start`) or email (`POST /api/v1/auth/login` / `/api/v1/auth/signup`). On hosted Syncidian.com, GitHub sign-in is hidden until the app name is tapped six times, and `SYNCIDIAN_GITHUB_ALLOWED_EMAIL` limits which GitHub accounts can complete OAuth. Operators use `/admin`. GitHub App install for vault backup happens after identity, on one repository, `main` only.

## Revocation

Revoke a token on the dashboard **Tokens** page. A revoked token returns `401` with `invalid or revoked access token`.
