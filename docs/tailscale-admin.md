# Private operator hostname (Tailscale)

## Self-host: skip this

You do **not** need Tailscale to run Syncidian.

```bash
docker compose up --build -d
# open http://localhost:8080/admin
```

Leave `SYNCIDIAN_ADMIN_HOST`, `SYNCIDIAN_ADMIN_LISTEN_IP`, and `TAILSCALE_IP` unset. To ignore a copied `.env` that already has those values:

```bash
SYNCIDIAN_ADMIN_PRIVATE=0
```

That keeps the operator UI at `/admin` on the same URL as the public site.

## Hosted Syncidian.com (Railway)

The production process is not on a Tailscale node. Leave `SYNCIDIAN_ADMIN_LISTEN_IP` and `TAILSCALE_IP` unset.

Tailscale’s DNS page has **no Extra records** control (it was never shipped). A second hostname such as `admin.syncidian.com` also needs a **second Railway custom domain**. Trial/hobby limits are often **one custom domain**; if Railway refuses `admin.syncidian.com`, keep `SYNCIDIAN_ADMIN_HOST` unset and open `https://syncidian.com/admin`.

To use `admin.syncidian.com` on Railway you need a plan that allows another domain, then one of the DNS options in [section 4](#4-dns-tailscale-has-no-extra-records). Until that domain is attached, setting `SYNCIDIAN_ADMIN_HOST` makes `/admin` on the public site return 404 and the operator UI is unreachable.

---

Hide the Syncidian operator UI from the public internet **only if you want that extra lock**. Vault users keep using the public site (`https://syncidian.com` or your public URL). Operators open **`https://admin.syncidian.com`** only while connected to your Tailscale mesh.

This is the right pattern behind **CGNAT** (no public IP, no port-forward, no public A-record for the admin name).

| Piece | Value |
| --- | --- |
| Public site | `SYNCIDIAN_PUBLIC_URL` (Cloudflare Tunnel, Railway, or a reverse proxy on loopback) |
| Operator hostname | `SYNCIDIAN_ADMIN_HOST=admin.syncidian.com` |
| Tailscale IPv4 | `SYNCIDIAN_ADMIN_LISTEN_IP` or `TAILSCALE_IP` (CGNAT `100.x.x.x`) |
| Docker publish | `SYNCIDIAN_BIND_IP` — same `100.x` so the host socket is not `0.0.0.0` |

Do **not** create a public DNS `A`, `AAAA`, or `CNAME` for `admin.syncidian.com`. GitHub App callback / setup / webhook URLs stay on the **public** origin.

---

## 1. Install Tailscale and read the 100.x address

### Ubuntu 24.04 / Debian

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
tailscale ip -4
```

Example output: `100.64.1.20`. Put that in an env file the Syncidian process can read:

```bash
echo "TAILSCALE_IP=$(tailscale ip -4)" | sudo tee /etc/syncidian-tailscale.env
```

`SYNCIDIAN_ADMIN_LISTEN_IP` wins if both are set. Either name is valid.

### Confirm the node is only on the mesh

```bash
tailscale status
ip -4 addr show tailscale0
ss -lnt | grep 8080 || true
```

`tailscale0` is a `100.x` address. Nothing here is a public WAN IP.

### Docker on Linux (host network)

If the container uses `network_mode: host`, it can bind `100.x` itself. Install Tailscale on the **host** (commands above), then pass `SYNCIDIAN_ADMIN_LISTEN_IP` into the container.

### Docker bridge (default Compose)

The container **cannot** bind the host’s Tailscale IP. Publish the port on that IP instead (`SYNCIDIAN_BIND_IP`) and keep `SYNCIDIAN_ADMIN_LISTEN_IP` empty. App-level `SYNCIDIAN_ADMIN_HOST` still rejects operator routes on the public hostname.

### Windows 11

Download Tailscale from [tailscale.com/download](https://tailscale.com/download), sign in, then in PowerShell:

```powershell
tailscale ip -4
```

Set a user environment variable `TAILSCALE_IP` to that value, or put `SYNCIDIAN_ADMIN_LISTEN_IP` in the service’s environment.

### Unraid

Apps → **Tailscale** (official plugin) → Settings → note **IPv4**. Add `SYNCIDIAN_ADMIN_LISTEN_IP` / `SYNCIDIAN_ADMIN_HOST` to the Syncidian container template. Prefer host network, or bind the port to the Tailscale interface IP.

### TrueNAS SCALE

Install the Tailscale app (or enable Tailscale on the host). In the Syncidian app env vars set `SYNCIDIAN_ADMIN_HOST` and `SYNCIDIAN_ADMIN_LISTEN_IP`. If the app is a jail/container without `tailscale0`, bind the host port to the Tailscale IP the same way as Docker bridge.

---

## 2. Bind so the operator name is not on 0.0.0.0

### Go process on the host (no Docker)

```bash
export SYNCIDIAN_PUBLIC_URL=https://syncidian.com
export SYNCIDIAN_ADMIN_HOST=admin.syncidian.com
export TAILSCALE_IP=$(tailscale ip -4)
# optional explicit name:
# export SYNCIDIAN_ADMIN_LISTEN_IP="$TAILSCALE_IP"

go run ./cmd/syncidian serve
# or: syncidian serve
```

The server then listens on **`127.0.0.1:8080`** (Cloudflare Tunnel / local Caddy) and **`$TAILSCALE_IP:8080`**. It does **not** listen on `0.0.0.0`. Logs look like:

```text
syncidian listening addr=127.0.0.1:8080,100.64.1.20:8080 admin_host=admin.syncidian.com
```

If Tailscale is still coming up, the process retries the `100.x` bind for about 30 seconds.

### Docker Compose

In `.env` (never commit the 100.x address if the repo is public):

```bash
SYNCIDIAN_PUBLIC_URL=https://syncidian.com
SYNCIDIAN_ADMIN_HOST=admin.syncidian.com
SYNCIDIAN_BIND_IP=100.64.1.20
```

```bash
docker compose up --build -d
ss -lnt | grep 8080
```

You should see `100.64.1.20:8080`, not `0.0.0.0:8080`. For a public site on the same host, also publish loopback:

```yaml
ports:
  - "127.0.0.1:8080:8080"
  - "${SYNCIDIAN_BIND_IP}:8080:8080"
```

### Host-network Compose (Linux, Tailscale on the host)

```yaml
services:
  syncidian:
    network_mode: host
    environment:
      SYNCIDIAN_ADDR: ":8080"
      SYNCIDIAN_PUBLIC_URL: https://syncidian.com
      SYNCIDIAN_ADMIN_HOST: admin.syncidian.com
      SYNCIDIAN_ADMIN_LISTEN_IP: ${TAILSCALE_IP}
```

No `ports:` mapping. The process binds loopback + Tailscale itself.

---

## 3. Reverse proxy: listen only on the Tailscale IP

If Caddy/Nginx/Nginx Proxy Manager is the TLS terminator, **bind that vhost to the Tailscale IP**. Do not add `admin.syncidian.com` to a server block that already listens on `0.0.0.0:443`.

### Caddy

Install Caddy on the host, then use [`deploy/caddy/Caddyfile.admin.example`](../deploy/caddy/Caddyfile.admin.example):

```caddy
{$SYNCIDIAN_ADMIN_HOST:admin.syncidian.com} {
	bind {$SYNCIDIAN_ADMIN_LISTEN_IP}
	reverse_proxy 127.0.0.1:8080
}
```

```bash
export SYNCIDIAN_ADMIN_HOST=admin.syncidian.com
export SYNCIDIAN_ADMIN_LISTEN_IP="$(tailscale ip -4)"
sudo caddy run --config deploy/caddy/Caddyfile.admin.example
```

`bind` is what stops the site from landing on `0.0.0.0`. Public `syncidian.com` stays on a **different** site block (or Cloudflare Tunnel to `127.0.0.1:8080` only).

HTTPS: HTTP-01 cannot complete for a name with no public A record. Use **DNS-01** (Cloudflare API token) for a real certificate, or Tailscale’s HTTPS certs on `*.ts.net` (see MagicDNS below).

### Nginx

```nginx
server {
    listen 100.64.1.20:443 ssl;
    server_name admin.syncidian.com;
    # ssl_certificate ... (DNS-01)
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

Replace `100.64.1.20` with `$TAILSCALE_IP`. There must be **no** `listen 443` / `listen 0.0.0.0:443` for this `server_name`.

### Nginx Proxy Manager

Do not use a public Proxy Host for `admin.syncidian.com`. Run NPM only if its UI/proxy bind can be limited to the Tailscale interface, or put Caddy/Nginx in front as above. A public Proxy Host with “don’t cache / websocket” still publishes the name on `0.0.0.0`.

### Apache

```apache
<VirtualHost 100.64.1.20:443>
    ServerName admin.syncidian.com
    SSLEngine on
    ProxyPass / http://127.0.0.1:8080/
    ProxyPassReverse / http://127.0.0.1:8080/
    RequestHeader set X-Forwarded-Proto "https"
</VirtualHost>
```

---

## 4. DNS: Tailscale has no Extra records

The [Tailscale DNS page](https://login.tailscale.com/admin/dns) has **MagicDNS**, **Nameservers**, and **Split DNS**. It does **not** have Extra records. Tailscale never shipped a UI to add arbitrary A/CNAME names to MagicDNS ([docs](https://tailscale.com/kb/1054/dns): “It's not possible to add arbitrary records to MagicDNS”).

Use one of the options below.

### Hosted on Railway (no Tailscale IP on the app)

The Syncidian process is not on a node with `tailscale0`. Leave `SYNCIDIAN_ADMIN_LISTEN_IP` and `TAILSCALE_IP` unset. A 100.x A record cannot reach Railway.

1. Add `admin.syncidian.com` as a **Railway custom domain** on the Syncidian service (same port as the public site, usually `8080`).
2. Resolve the name **only on operator machines**. Pick one:

   **A. Hosts file (no extra Tailscale UI)** — on each operator laptop, after Railway shows the target:

   ```text
   # Linux / macOS: /etc/hosts
   # Windows: C:\Windows\System32\drivers\etc\hosts
   <Railway edge IPv4> admin.syncidian.com
   ```

   `dig +short <your-service>.up.railway.app` (or the CNAME Railway prints) gives that IPv4. Only machines with this line will open the name.

   **B. Public CNAME at Cloudflare / registrar** — add the CNAME and TXT Railway prints. The hostname then works from any network. Protection is `SYNCIDIAN_ADMIN_HOST` plus the admin password, not the mesh.

   **C. Split DNS + Pi-hole / AdGuard** (only if you already run one on the tailnet):

   1. [DNS](https://login.tailscale.com/admin/dns) → **Nameservers** → **Add nameserver** → **Custom**.
   2. Choose **Restrict to domain** (Split DNS). Domain: `admin.syncidian.com`. Nameserver: the Pi-hole’s Tailscale IPv4.
   3. On the Pi-hole, add a local A record `admin.syncidian.com` → the Railway edge IPv4 (same value as the hosts file).

   Do **not** split the whole `syncidian.com` zone unless that nameserver also answers the public site correctly.

3. Railway still needs its verification TXT (and usually a CNAME) to issue HTTPS. If you skip the public CNAME and use only a hosts file, expect certificate errors until Railway can complete domain verification.

### Self-host on a Tailscale machine

Something is listening on `100.x:443` or `100.x:8080`. Tailscale’s documented approach is a **public** A record that points at the mesh address. Anyone can look the name up; only the tailnet can connect.

| Name | Type | Value |
| --- | --- | --- |
| `admin.syncidian.com` | `A` | `100.64.1.20` (your `tailscale ip -4`) |

Add that at Cloudflare / registrar. Do not proxy it through Cloudflare orange-cloud (TCP to `100.x` must stay direct).

On a device already on the tailnet:

```bash
tailscale status
dig +short admin.syncidian.com
# or: getent hosts admin.syncidian.com
```

The answer must be the Tailscale IPv4. From a phone **off** Tailscale, `curl` must fail to connect even if DNS resolves.

### Option: MagicDNS hostname instead of a custom name

If you do not need `admin.syncidian.com` and Syncidian **runs on a Tailscale node**:

```text
https://<machine>.<tailnet>.ts.net
```

Set `SYNCIDIAN_ADMIN_HOST` to that MagicDNS name (no `https://`). Tailscale can issue HTTPS for `*.ts.net`. This is the simplest mesh-only hostname.

### Option: Pi-hole / local split-horizon (self-host)

On a Pi-hole that **only** Tailscale clients use (Pi-hole advertised as a Tailscale nameserver, or LAN DNS that is not public):

```text
# /etc/pihole/custom.list  (or Local DNS → DNS Records)
100.64.1.20 admin.syncidian.com
```

```bash
pihole restartdns
dig @<pihole-tailscale-ip> admin.syncidian.com
```

Then in Tailscale: **Nameservers** → **Add nameserver** → **Restrict to domain** `admin.syncidian.com` → the Pi-hole’s `100.x`. Do not point **public** recursive DNS at this Pi-hole.

---

## 5. Open the operator site

On a laptop with Tailscale connected:

```bash
curl -sS https://admin.syncidian.com/health
# or http://admin.syncidian.com:8080/health if you skipped TLS
```

Create the first admin there (not on the public landing). Vault users continue to sign in at the public URL.

From a network **without** Tailscale:

```bash
curl -sS -o /dev/null -w "%{http_code}\n" --connect-timeout 5 https://admin.syncidian.com/health
```

This should fail to connect (no public listener, no public DNS). `https://syncidian.com/admin` returns **404** when `SYNCIDIAN_ADMIN_HOST` is set.

---

## 6. What Syncidian does in software

- `SYNCIDIAN_ADMIN_HOST` — operator SPA and admin APIs (`POST /api/v1/setup`, admin login, user admin, GitHub App register, waitlist list) require that `Host` header. `X-Forwarded-Host` is ignored for this check.
- `SYNCIDIAN_ADMIN_LISTEN_IP` / `TAILSCALE_IP` — drop `0.0.0.0`, listen on loopback + that unicast IP.
- GitHub App URLs copied on the operator page still use `SYNCIDIAN_PUBLIC_URL` so GitHub can reach callback / setup / webhook.
- `robots.txt` on the public site does not mention `/admin` when a private admin host is configured.

Path-based `/admin` (or `SYNCIDIAN_ADMIN_PATH`) is the self-host default when `SYNCIDIAN_ADMIN_HOST` is unset, or when `SYNCIDIAN_ADMIN_PRIVATE=0`. `TAILSCALE_IP` in the environment is ignored unless `SYNCIDIAN_ADMIN_HOST` is also set.
