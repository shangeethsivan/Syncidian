# Community plugin directory (mobile)

This repository is a **monorepo**. Packaging, tagging, and submitting to [community.obsidian.md](https://community.obsidian.md) are documented in the root [`README.md`](../README.md) (`# 📦 Publishing the Obsidian plugin`). Use `make plugin-manifest` and `.github/workflows/release.yml`.

Phone install steps: [`docs/install-mobile.md`](install-mobile.md).

This page is the **mobile** checklist so that listing is not desktop-only, plus where plugin files actually live.

## Two different `manifest.json` roles (not a tests folder)

Nothing is generated into `tests/`. `internal/server/server_test.go` (`TestCommunityPluginManifestAtRepoRoot`) only **asserts** that the files below exist and match. The plugin Obsidian loads is never written there.

| What | Where | Who reads it |
| --- | --- | --- |
| Community-directory metadata | **`manifest.json` and `versions.json` at the git repo root** (copies of `plugin/manifest.json` and `plugin/versions.json`, via `make plugin-manifest`) | Obsidian’s **community.obsidian.md** crawler, at HEAD of the default branch |
| Plugin source and the files you sideload | **`plugin/`** (`main.ts`, compiled `main.js`, `manifest.json`, `styles.css`) | You, `scripts/install-plugin.sh`, and the release workflow |
| Running server copies | **`internal/web/static/assets/obsidian/`** (same three files, via `make plugin-manifest`) | `GET /assets/obsidian/` and `GET /assets/obsidian.zip` so you can update a vault without cloning the repo |
| GitHub Release **Assets** | Attachments on the release whose **tag equals** `version` (`0.1.0`, no `v` prefix) | **BRAT** and in-app Community plugins. Exactly three files: `main.js`, `manifest.json`, `styles.css` |

**“Assets” on a GitHub Release** means the downloadable files listed under the release title (not git LFS, not the source zip GitHub always adds). If that list has only “Source code (zip)” and no `main.js`, BRAT has nothing to install.

A tag or a release **title** is not enough. The workflow in `.github/workflows/release.yml` must **upload** those three files. Creating a release by hand in the GitHub UI does not attach them unless you upload them yourself.

If the repo is **private**, GitHub build attestations are not available. The workflow skips attestation on private repos so upload still runs. After this workflow is on the default branch, re-attach files to an existing tag from **Actions → Release Obsidian plugin → Run workflow** and enter the tag (for example `0.1.0`).

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
- On a phone, leave Syncidian.com selected or set Custom Domain to public HTTPS. `localhost` is the device, not the computer.
