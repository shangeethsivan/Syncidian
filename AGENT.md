# Agent instructions for Syncidian

This file tells coding agents how to keep documentation honest when the product changes.

## When architecture or user-flow changes

If you change any of the following, **update the docs in the same change**:

* Authentication or the landing page (what an unauthenticated visitor sees)
* When GitHub is configured (must stay **after login**, **per user**)
* Admin vs regular-user capabilities and privacy
* HTTP routes, data model, sync plan, MCP, or deployment topology
* Multi-user isolation or token/session behavior

Do not ship a behavioral change that leaves `README.md`, `docs/github-app.md`, or `docs/architecture.md` describing the old flow.

## Diagrams you must keep in sync

GitHub-rendered Mermaid diagrams are part of the product docs. After an architectural change, update every diagram that would otherwise be wrong:

| Diagram | Location |
| --- | --- |
| App workflow (public landing → GitHub or email → optional per-user repo) | `README.md` (`# 🗺️ App workflow`) and `docs/architecture.md` (section 3b) |
| System overview | `README.md` (Vision + Architecture) and `docs/architecture.md` §1 |
| HTTP surface (public vs session vs plugin) | `docs/architecture.md` §3 |
| Plugin first sync / edit / conflicts | `docs/architecture.md` §4–7 |
| Authentication | `docs/architecture.md` §8 |
| MCP | `docs/architecture.md` §9 |
| Data model (`github_config.user_id`) | `docs/architecture.md` §10 |
| Multi-user isolation and admin privacy | `docs/architecture.md` §11 |
| Deployment | `docs/architecture.md` §12 |

If a diagram no longer matches the code, rewrite it. Do not leave comments like “TODO: update diagram.” Prefer editing the existing mermaid block over adding a second conflicting one.

## Current invariants (do not regress)

Document and implement these unless a later change explicitly replaces them — and if you replace them, update this list too:

1. **Public landing, admin at `/admin`.** Unauthenticated visitors see a one-page story with GitHub sign-in, optional email signup, and a link to `/admin`. Operators create the first admin and register the instance GitHub App at `/admin`. Repository install stays per user after identity.
2. **GitHub is per user, after identity.** One repository per `user_id`. `GET/POST /api/v1/github` requires `authenticate()`.
3. **Admin login does not need repo sync.** Admins manage users. Vault/GitHub/token/device/activity/MCP routes use `vaultAuthed` and return 403 for admins.
4. **Admin user list is public fields only:** `username`, `is_admin`, `created_at` (no `id`, vault, tokens, or GitHub).
5. Device sync works without GitHub. GitHub is optional durable backup.
6. **GitHub App only, main branch only.** Backup is authorized by installing a GitHub App with Contents read and write. Personal access tokens and deploy keys are not supported. Syncidian always uses `main`. Sign-in uses the instance App callback, setup, and webhook URLs.

## How to update diagrams

* Keep Mermaid valid for GitHub (no HTML in node labels beyond `<br/>`).
* Name actors and routes the way the code does (`/api/v1/auth/login`, `sk_sync_…`, `github_config`).
* If you add a new package, route group, or table, add it to the relevant diagram and the “Related code” table in `docs/architecture.md`.
* If you remove a flow (for example a pre-login GitHub popup), delete it from every diagram and from Help copy in `internal/web/static/index.html`.

## Tests

When you change auth, GitHub scoping, or admin privacy, extend `internal/server/server_test.go` and `internal/store/store_test.go` rather than only updating prose.

## Help walkthrough

The dashboard **Help** button must describe the same workflow as the diagrams: public landing with GitHub sign-in, optional email, admin at `/admin`, register the instance GitHub App (`docs/github-app.md`), then optional per-user GitHub, admin without repo sync.
