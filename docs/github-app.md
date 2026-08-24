# Set up the GitHub App (self-host)

Anyone running their own Syncidian instance needs **one GitHub App** for that instance. Vault users then sign in with GitHub and install that app on **one** repository. Syncidian always uses the `main` branch. Personal access tokens and deploy keys are not used.

Do this after the server is reachable at a URL you will keep (for example `https://syncidian.example.com` or `http://localhost:8080` for a private laptop).

The operator UI is **`/admin`**, not the public landing page.

---

## What you are creating

GitHub asks for three URLs. Replace `{base}` with your public origin, with no trailing slash:

| GitHub field | URL | Why GitHub asks for it |
| --- | --- | --- |
| **Callback URL** (User authorization callback URL / redirect URI) | `{base}/api/v1/auth/github/callback` | After **Sign in with GitHub**, GitHub sends the browser here. |
| **Setup URL** | `{base}/api/v1/github/app/setup` | After someone **installs** the app, GitHub sends them here so Syncidian can bind that installation to their account. |
| **Webhook URL** | `{base}/api/v1/github/app/webhook` | GitHub requires a webhook URL so it can **ping** the app when you create or update it. Syncidian answers that ping with HTTP 200. You do not need extra webhook events for backup to work. |

Copy the filled-in values from `/admin` after you create the first admin, or from:

```text
GET {base}/api/v1/github/app/urls
```

Set `SYNCIDIAN_PUBLIC_URL` to `{base}` if the server sits behind a reverse proxy, so the URLs on `/admin` match the hostname people actually use.

---

## Option A — recommended: create it from `/admin`

This posts a GitHub App **manifest**, so permissions and URLs are filled in for you.

1. Start Syncidian (Docker Compose, `go run ./cmd/syncidian serve`, or your host).
2. Open `{base}/admin`.
3. Create the first admin (username + password of at least 8 characters).
4. On the admin overview, click **Create GitHub App**.
5. GitHub opens **Create a new GitHub App**. Review the name if you want, then click **Create GitHub App**.
6. GitHub redirects back to Syncidian. `/admin` should show the app as **Registered**.

People can now use **Sign up using GitHub** / **Log in** / **Connect to your GitHub repository** on `{base}/`.

The manifest requests:

* Repository permissions: **Contents** read and write, **Metadata** read
* Account permissions: **Email addresses** read (so sign-in can store an email)
* **Request user authorization (OAuth) during installation**: on
* Webhook: active, URL as above, event `push` (unused for the ping; backup uses installation tokens, not webhook payloads)

---

## Option B — create the app by hand, then point Syncidian at it

Use this when GitHub’s manifest flow is blocked, or you want the credentials in environment variables instead of the server database.

### 1. Create the GitHub App

1. Sign in to GitHub as the account or organization that should **own** the app.
2. Open [GitHub Apps](https://github.com/settings/apps) → **New GitHub App** (organization: Settings → Developer settings → GitHub Apps).
3. **GitHub App name:** something like `Syncidian` (must be unique on GitHub).
4. **Homepage URL:** `{base}`
5. **Callback URL:** `{base}/api/v1/auth/github/callback`  
   Expire user authorization tokens: optional.
6. Check **Request user authorization (OAuth) during installation**.
7. **Setup URL:** `{base}/api/v1/github/app/setup`  
   Check **Redirect on update** if GitHub shows it, so changing an install also returns to Syncidian.
8. **Webhook:** active. **Webhook URL:** `{base}/api/v1/github/app/webhook`. Secret: optional (Syncidian does not verify a secret today).
9. **Repository permissions:**
   * **Contents:** Read and write
   * **Metadata:** Read-only (required)
10. **Account permissions:**
    * **Email addresses:** Read-only
11. **Where can this GitHub App be installed?**
    * **Only on this account** — fine if you are the only GitHub user who will install it.
    * **Any account** — required if other people on this Syncidian instance must install it on **their** GitHub users or orgs.
12. Click **Create GitHub App**.

### 2. Copy credentials

On the app’s settings page:

1. Note **App ID** (number) and the slug in the URL (`https://github.com/apps/<slug>`).
2. **Client ID** is shown on the page. **Generate a new client secret** and copy it once.
3. **Private keys** → **Generate a private key**. Download the `.pem` file.

### 3. Give the credentials to Syncidian

**Environment variables** (restart the process after setting them):

```bash
export SYNCIDIAN_PUBLIC_URL="https://syncidian.example.com"
export SYNCIDIAN_GITHUB_APP_ID="123456"
export SYNCIDIAN_GITHUB_APP_SLUG="syncidian"
export SYNCIDIAN_GITHUB_CLIENT_ID="Iv1.xxxxxxxx"
export SYNCIDIAN_GITHUB_CLIENT_SECRET="xxxxxxxx"
# Literal \n sequences are turned into real newlines:
export SYNCIDIAN_GITHUB_APP_PRIVATE_KEY="$(sed ':a;N;$!ba;s/\n/\\n/g' /path/to/syncidian.private-key.pem)"
```

Docker Compose can pass the same names through. A one-line PEM:

```yaml
environment:
  SYNCIDIAN_GITHUB_APP_ID: "123456"
  SYNCIDIAN_GITHUB_APP_SLUG: "syncidian"
  SYNCIDIAN_GITHUB_CLIENT_ID: "Iv1.xxxxxxxx"
  SYNCIDIAN_GITHUB_CLIENT_SECRET: "xxxxxxxx"
  SYNCIDIAN_GITHUB_APP_PRIVATE_KEY: "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----"
```

Env vars override a GitHub App stored in the database.

Keep the `.pem` and client secret off the public site. `/admin` never shows another user’s vault; it also should not print these secrets.

---

## After the app exists: what each vault user does

Device sync works with no GitHub at all. GitHub is optional backup, one repo per user.

1. Open `{base}/` (not `/admin`).
2. Click **Sign up using GitHub** or **Connect to your GitHub repository**.
3. Authorize the app, then **Install** it on **one** repository (or several, then pick one in the dashboard).
4. Syncidian always commits to **`main`**. Other branches are not used.
5. Create a `sk_sync_…` token on that user’s **Tokens** page and paste it into the Obsidian plugin. The plugin never sees GitHub credentials.

If GitHub sent them to the **Setup URL** after install, Syncidian records the installation and returns them to the dashboard.

---

## Localhost and tunnels

* **Callback and setup** only need to work in the **browser** that signs in. `http://localhost:8080/...` is valid for a single-machine install.
* **Webhook pings** come from GitHub’s servers. They cannot reach `localhost`. You can still create the app; GitHub may show failed deliveries. Sign-in and `git` backup do not wait on those pings.
* For a phone or another PC, use a real HTTPS hostname (or a tunnel) as `{base}` and put that same origin in the three URLs. Then click **Create GitHub App** again, or edit the existing app’s URLs on GitHub to match.

---

## Checklist when something fails

| Symptom | Likely cause |
| --- | --- |
| “This instance has no GitHub App yet” | Finish Option A or B. Confirm `/admin` shows **Registered**, or the `SYNCIDIAN_GITHUB_APP_*` variables are set. |
| OAuth error / redirect mismatch | Callback URL on the GitHub App must be exactly `{base}/api/v1/auth/github/callback`. `{base}` must be the origin in the address bar (scheme + host + port). |
| Install succeeds but Syncidian says credentials are missing | Setup URL must be `{base}/api/v1/github/app/setup`. Sign in as a **non-admin** vault user; admins do not connect a repo. |
| Cannot install on someone else’s GitHub account | Make the app installable on **Any account** (public to GitHub users, not the Marketplace). |
| Push/pull fails | Contents must be **read and write**. The install must include the chosen repo. Branch must be `main`. |

Admins never connect a vault repository and cannot see another user’s GitHub credentials.
