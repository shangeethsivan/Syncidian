# Syncidian

Your data stays with you. Notes on every device. One GitHub repository you own.

Syncidian is a self-hosted sync server and MCP bridge for Obsidian. The same plugin runs on Windows, macOS, Linux, Android, and iOS. GitHub is optional backup: after you sign in, you can install the instance GitHub App on one repository. The plugin never sees GitHub credentials.

## Sign in

- **Waitlist (Syncidian.com):** use the form on the [homepage](/). Hosted GitHub sign-in is hidden until you tap the app name six times, and only allowlisted emails can complete it until public launch.
- **GitHub (self-host, or after revealing it):** [Continue with GitHub](/api/v1/auth/github/start?next=install)
- **Email:** use the Email button on the [homepage](/)

## For AI agents

- MCP JSON-RPC: `POST /mcp` with `Authorization: Bearer sk_sync_…`
- How to get a token: [auth.md](/auth.md)
- MCP Server Card: [/.well-known/mcp/server-card.json](/.well-known/mcp/server-card.json)
- API catalog: [/.well-known/api-catalog](/.well-known/api-catalog)
- OpenAPI: [/openapi.json](/openapi.json)
- Health: [GET /health](/health)

MCP does not store note bodies on this server. Read and write tools use the user's connected GitHub repository. Search and read are on by default; create and modify stay off until the dashboard enables them.

## After you sign in

1. Connect one GitHub repository (optional, required for MCP writes).
2. Create an access token (`sk_sync_…`) on **Tokens**.
3. Install the Syncidian Obsidian plugin and paste the token. Download `main.js`, `manifest.json`, and `styles.css` from [this server](/assets/obsidian.zip) (or `/assets/obsidian/`) without cloning the git repo.

Source: [github.com/shangeethsivan/Syncidian](https://github.com/shangeethsivan/Syncidian)
