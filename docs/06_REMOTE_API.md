# Remote API access (iPad / VPN)

PatentMine’s HTTP API (`patentmine api`) is a thin client to the daemon. For remote use:

## 1. Start processes

```bash
patentmine serve
patentmine api --addr 0.0.0.0:18080 --web-dir ./web/dist
```

## 2. Authentication

Set a shared secret:

```bash
export PATENTMINE_API_TOKEN="your-long-random-token"
```

Clients send `Authorization: Bearer <token>` on every request except `GET /healthz`.

`EventSource` (`GET /events`) cannot set headers; pass the same token as `?token=` on that URL only.

## 3. CORS (browser / iPad Safari)

```bash
export PATENTMINE_API_CORS_ORIGINS="https://your-host.example,http://127.0.0.1:5173"
```

## 4. TLS

**Recommended:** terminate TLS with Caddy or nginx in front of the API.

**Optional:** direct TLS on the API process:

```bash
export PATENTMINE_API_TLS_CERT=/path/to/fullchain.pem
export PATENTMINE_API_TLS_KEY=/path/to/privkey.pem
```

## 5. Web UI

Build the SPA:

```bash
cd web && npm install && npm run build
```

Open `https://your-host:18080/ui/` (hash routes under `#/`).

Store the API token under **Settings** in the UI (localStorage).

## Example Caddyfile

```text
your-host.example {
  reverse_proxy 127.0.0.1:18080
}
```

Place TLS certificates via Caddy’s automatic HTTPS or your own certs.
