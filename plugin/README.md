# Syncidian (Obsidian plugin)

Syncidian is an Obsidian community plugin that syncs a vault through a self-hosted Syncidian server. It is **not desktop-only**. The same plugin runs on Windows, macOS, Linux, **Android**, and **iOS**.

Git-based community plugins (for example [Obsidian Git](https://github.com/denolehov/obsidian-git)) set `isDesktopOnly: true` because they call Node.js, Electron, or a local `git` binary. Those APIs do not exist in the Android and iOS apps, so those plugins never appear on mobile. Syncidian never shells out to Git and never imports Node or Electron. Vault I/O uses the Obsidian Vault API. HTTP uses Obsidian `requestUrl` (no CORS). Live updates use WebSockets with an HTTP poll fallback.

## Install

Full Android and iOS steps (Restricted mode, Community plugins, BRAT, copying files, Connect): **[docs/install-mobile.md](../docs/install-mobile.md)**.

**From Community plugins** (once listed): Settings → Community plugins → turn **Restricted mode** off → Browse → **Syncidian** → Install → Enable.

Until then:

1. From a running Syncidian server, download `/assets/obsidian.zip` (or the three files under `/assets/obsidian/`) into `{Vault}/.obsidian/plugins/syncidian/`. That is the fastest way to refresh a local install after a deploy: `./scripts/install-plugin.sh "/path/to/YourVault" "https://your-syncidian.example"`.
2. Or copy `plugin/manifest.json`, `plugin/main.js`, and `plugin/styles.css` from this repo, or `./scripts/install-plugin.sh "/path/to/YourVault"`.
3. In Obsidian: Settings → Community plugins → turn **Restricted mode** off → enable **Syncidian**.

Those three files live in **`plugin/`** in this repo. They are not written to a `tests/` folder.

[BRAT](https://github.com/TfTHacker/obsidian42-brat): add `shangeethsivan/Syncidian` only after a GitHub Release whose tag matches `manifest.json` `version` **and** whose **Assets** include `main.js`, `manifest.json`, and `styles.css`. A tag without those attachments is not enough. See [docs/community-plugin.md](../docs/community-plugin.md).

On a phone, **Server URL** must be a public `https://` address. `http://localhost:8080` is the phone itself, not your computer. iOS often blocks plain `http://`.

## Settings

- **Server URL** — your Syncidian instance (`https://sync.example.com`).
- **Access token** — a vault-user `sk_sync_…` token from the dashboard (not an admin account).
- **Device name** — filled in from the Obsidian `Platform` API (Android, iPhone, iPad, macOS, …).

GitHub backup is configured in the web dashboard, not in this plugin.

## Mobile requirements (implemented)

| Requirement | How Syncidian meets it |
| --- | --- |
| `isDesktopOnly: false` in `manifest.json` | Set. Community review will load the plugin on a phone. |
| No Node.js / Electron APIs | Only `obsidian` is imported. CI fails if `main.js` requires `fs`, `child_process`, `electron`, etc. |
| HTTP from the WebView | `requestUrl` instead of `fetch`, so Android (`http://localhost` origin) and iOS (`capacitor://localhost`) are not blocked by CORS. |
| Status bar missing on phones | Ribbon icon shows sync status; tap to sync now. |
| WebSocket blocked (cleartext / ATS) | HTTP poll of `/api/v1/sync/manifest` every 15s while the socket is down. |
| App backgrounded (desktop or phone) | On window focus, `visibilitychange`, `online`, or Capacitor `resume`, poll the manifest even if a WebSocket still looks open. |
| Community directory `manifest.json` | Copied at the **repository root** (required) as well as `plugin/manifest.json`. |
| BRAT / in-app install | GitHub Release **Assets**: `main.js`, `manifest.json`, `styles.css` (not a tests folder). |

## Develop

```bash
cd plugin && npm install && npm run build
```

Commit `plugin/main.js` so people can sideload without Node.js. Keep root `manifest.json` identical to `plugin/manifest.json`.

Publishing to the Community Plugin directory: [docs/community-plugin.md](../docs/community-plugin.md).
