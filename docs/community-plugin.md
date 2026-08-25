# Community plugin directory (mobile)

This repository is a **monorepo**. Packaging, tagging, and submitting to [community.obsidian.md](https://community.obsidian.md) are documented in the root [`README.md`](../README.md) (`# 📦 Publishing the Obsidian plugin`). Use `make plugin-manifest` and `.github/workflows/release.yml`.

This page is the **mobile** checklist so that listing is not desktop-only.

## Keep `isDesktopOnly` false

Git-style plugins (for example Obsidian Git) set `isDesktopOnly: true` because they call Node.js, Electron, or a local `git` binary. Those APIs do not exist in the Android and iOS apps.

Syncidian must stay `isDesktopOnly: false`. The plugin uses:

- Obsidian `requestUrl` for HTTP (not `fetch`, which CORS-blocks the mobile WebView)
- The Vault API (not Node `fs`)
- `Platform` for device names
- A ribbon status (the status bar is hidden on phones)
- HTTP poll of `/api/v1/sync/manifest` when WebSockets fail

CI fails if `plugin/main.js` `require()`s Node or Electron modules.

## Review notes

- Plugin source lives in [`plugin/`](../plugin/). Entry: `main.ts`. Helpers: `hash.ts`, `codec.ts`, `mobile.ts`.
- Description is sentence case, ends with a period, no emoji, under 250 characters.
- Settings UI has no extra “Syncidian” heading.
- On a phone, Server URL must be public HTTPS. `localhost` is the device, not the computer.
