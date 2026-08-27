# Enable Syncidian on Android and iOS

The Syncidian plugin is **not desktop-only**. The same `syncidian` plugin runs in Obsidian on Windows, macOS, Linux, Android, and iOS.

You still need a running Syncidian **server** (Railway, Docker, or similar) and a vault-user token (`sk_sync_…`) from that server’s dashboard. The plugin never talks to GitHub itself.

On a phone, **Server URL must be a public `https://` address** (for example your Railway domain). `http://localhost:8080` is the phone, not your computer. iOS often blocks plain `http://`.

## Turn Restricted mode off (required on every device)

Obsidian ships with community plugins blocked until you turn **Restricted mode** off.

1. Open the vault in Obsidian.
2. Open **Settings** (gear).
3. Tap **Community plugins**.
4. Tap **Turn off restricted mode** (or toggle Restricted mode **off**). Confirm if asked.
5. You should now see **Installed plugins** and a **Browse** button.

Until Restricted mode is off, Syncidian will not appear, even if the files are already in the vault.

## Option A — Community plugins (easiest, once listed)

1. Restricted mode off (above).
2. **Browse** → search **Syncidian** → **Install** → **Enable**.
3. Continue at [Connect the plugin](#connect-the-plugin).

## Option B — BRAT (before the Community listing, after a GitHub Release has files)

BRAT downloads **three files attached to a GitHub Release**: `main.js`, `manifest.json`, and `styles.css`. It does **not** install from the `tests/` folder or from the repo tree.

The release **tag** must match `manifest.json` `version` exactly (`0.1.0`, not `v0.1.0`). Those three files must show up under the release’s **Assets** list on GitHub. If Assets is empty, BRAT cannot install — see [docs/community-plugin.md](community-plugin.md).

1. Restricted mode off.
2. Browse → install **Obsidian42 - BRAT** → Enable.
3. BRAT settings → **Add Beta plugin** → `shangeethsivan/Syncidian`.
4. Enable **Syncidian** on the Community plugins list.
5. Continue at [Connect the plugin](#connect-the-plugin).

## Option C — Copy the three plugin files into the vault

Use this when Community plugins and BRAT are not available yet, or when you want the files from a git checkout.

Obsidian loads **exactly these three files** from:

```text
{Vault}/.obsidian/plugins/syncidian/manifest.json
{Vault}/.obsidian/plugins/syncidian/main.js
{Vault}/.obsidian/plugins/syncidian/styles.css
```

Get them from `plugin/` in this repository (after `cd plugin && npm run build` if you changed TypeScript). They are **not** generated into a `tests/` folder. A Go test only *checks* that `manifest.json` and `versions.json` also exist at the **repository root** for Obsidian’s community directory.

`.obsidian` is a hidden folder. Turn on **Show hidden files** in the Files app if you cannot see it.

### Android

1. Install Obsidian from the Play Store and open (or create) the vault you want to sync.
2. Copy the three files into that vault’s `.obsidian/plugins/syncidian/` folder. Typical ways:
   - Copy the folder from a desktop that already has the plugin (USB, Google Drive, Syncthing, a shared vault).
   - On the phone: Files app → the vault folder (often under Documents or a folder you chose when creating the vault) → `.obsidian` → `plugins` → create `syncidian` → paste the three files.
3. In Obsidian: **Settings → Community plugins → Restricted mode off**.
4. Pull down to refresh the installed list if needed, then enable **Syncidian**.
5. Continue at [Connect the plugin](#connect-the-plugin).

If Syncidian does not appear: confirm the folder is named `syncidian` (plugin `id`), not `Syncidian`, and that `manifest.json` sits inside that folder, not one level up.

### iOS / iPadOS

1. Install Obsidian from the App Store. Prefer a vault in **iCloud Drive** so you can drop files from a Mac.
2. Put the three files in `{Vault}/.obsidian/plugins/syncidian/`:
   - **From a Mac:** copy them into the same vault path Obsidian uses on iCloud (`iCloud Drive/Obsidian/YourVault/...`), then wait for iCloud to finish.
   - **On the device:** Files app → **iCloud Drive** → **Obsidian** → your vault → `.obsidian` → `plugins` → `syncidian`.
3. Fully close and reopen Obsidian if the plugin list is stale.
4. **Settings → Community plugins → Restricted mode off** → enable **Syncidian**.
5. Continue at [Connect the plugin](#connect-the-plugin).

## Connect the plugin

1. **Settings → Syncidian**.
2. **Server URL:** `https://your-public-host` (the same origin as the web dashboard). Do not use `localhost` on a phone.
3. **Access token:** a vault-user `sk_sync_…` token from the dashboard (Tokens page). Admins cannot sync as admin.
4. **Device name:** filled in from the device (Android, iPhone, iPad). You can rename it.
5. Tap **Connect**.

The ribbon icon shows sync status; tap it to sync now. The status bar is hidden on phones.

If Connect fails: the token is wrong or from another server, the URL is not reachable from the phone (try the dashboard in Safari/Chrome on the same device), or iOS is blocking HTTP.
