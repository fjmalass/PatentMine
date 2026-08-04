# Build / Deploy Secret Architecture

PatentMine supports two operating modes:

- **Local workstation:** one machine runs the daemon, TUI, and optional web API.
- **Build/deploy split:** an operator/build machine manages secrets and release
  material; a deploy machine or VPS runs `patentmine serve`; TUI/GUI clients talk
  to that daemon.

The build/deploy split is the preferred architecture for shared or remote use.
It keeps high-privilege operator secrets off the daemon and makes the TUI/GUI thin
clients: they can request work from the daemon but never read raw credential files.

---

## 1. Architecture

```mermaid
flowchart LR
    subgraph Build[Build / Operator Machine]
        A[local admin secrets]
        B[VPS mirror of deploy secrets]
        C[release / config preparation]
    end

    subgraph VPS[Deploy Machine / VPS]
        D[patentmine serve]
        E[/etc/patentmine/secrets]
        F[patentmine.db + logs]
    end

    subgraph Clients[TUI / GUI Clients]
        G[patentmine tui]
        H[web GUI / REST API]
    end

    A --> C
    B --> C
    C -->|deploy files, not admin-only secrets| E
    E --> D
    F --> D
    G -->|RPC / SSH tunnel / API| D
    H -->|HTTP API / reverse proxy| D
```

Rules:

- The daemon reads runtime secrets from its own deploy secret directory.
- TUI and GUI use daemon RPC/API calls; they do not read secret files directly.
- API responses expose redacted status only: configured/missing/auth-failed, never
  the key value.
- Build-host-only write/admin keys do not get copied to the VPS unless a daemon
  feature explicitly needs them.

---

## 2. Directory Layout

Recommended build machine layout:

```text
~/.ssh/patentmine/
  local/                         # build-host-only/admin secrets
  vps/
    etc/patentmine/secrets/      # mirror of deploy runtime secrets
```

Recommended deploy/VPS layout:

```text
/etc/patentmine/
  patentmine.env                 # runtime environment file
  secrets/
    uspto_odp_key
    epo_ops_consumer_key
    epo_ops_consumer_secret
    smtp_password
```

Recommended PatentMine home on a VPS:

```text
/var/lib/patentmine/
  patentmine.db
  logs/
  patents/
  exports/
```

Permissions:

```bash
sudo mkdir -p /etc/patentmine/secrets /var/lib/patentmine
sudo chown -R patentmine:patentmine /etc/patentmine /var/lib/patentmine
sudo chmod 700 /etc/patentmine/secrets
sudo chmod 600 /etc/patentmine/secrets/*
```

---

## 3. Runtime `.env`

The deploy `.env` should contain configuration and `file:` references, not raw
secret values.

Example `/etc/patentmine/patentmine.env`:

```dotenv
PATENTMINE_HOME=/var/lib/patentmine
PATENTMINE_CREDENTIALS_DIR=/etc/patentmine/secrets
PATENTMINE_SOURCE_MODE=uspto-first

PATENTMINE_USPTO_API_KEY=file:${PATENTMINE_CREDENTIALS_DIR}/uspto_odp_key

PATENTMINE_EPO_OPS_CONSUMER_KEY=file:${PATENTMINE_CREDENTIALS_DIR}/epo_ops_consumer_key
PATENTMINE_EPO_OPS_CONSUMER_SECRET=file:${PATENTMINE_CREDENTIALS_DIR}/epo_ops_consumer_secret

PATENTMINE_REMINDER_EMAIL_ENABLED=false
PATENTMINE_REMINDER_SMTP_PASSWORD=file:${PATENTMINE_CREDENTIALS_DIR}/smtp_password
```

For local development, the same shape works with a home-directory secret store:

```dotenv
PATENTMINE_HOME=~/.config/patentmine
PATENTMINE_CREDENTIALS_DIR=~/.ssh/patentmine/local
PATENTMINE_USPTO_API_KEY=file:${PATENTMINE_CREDENTIALS_DIR}/uspto_odp_key
PATENTMINE_EPO_OPS_CONSUMER_KEY=file:${PATENTMINE_CREDENTIALS_DIR}/epo_ops_consumer_key
PATENTMINE_EPO_OPS_CONSUMER_SECRET=file:${PATENTMINE_CREDENTIALS_DIR}/epo_ops_consumer_secret
```

`patentmine paths` prints the resolved home, database, socket, patent cache, and
credential directory so operators can confirm where the daemon is reading from.

---

## 4. Secret Classes

Runtime deploy secrets are credentials the daemon needs to perform normal work:

- USPTO ODP API key.
- EPO OPS consumer key and secret for EP validation/legal-status lookup.
- SMTP password when reminder email is enabled.
- Backup credentials only if the daemon performs backups.

Build-host-only/admin secrets are credentials that should normally stay off the
VPS:

- Payment write keys or payment provider admin keys.
- Cloud account owner keys.
- Deployment SSH private keys.
- Any credential capable of deleting infrastructure or moving money.

For renewal payments, the first implementation should keep the daemon in
**assisted/manual payment mode**: fetch fee/validation data, open official payment
sites, track receipt metadata, and mark payment complete after user review. Do
not place auto-payment write keys on the VPS until a dedicated auto-payment flow
exists with explicit controls.

---

## 5. VPS Daemon Access

Common access patterns:

- **SSH TUI:** SSH into the VPS and run `patentmine tui`; it connects to the local
  daemon socket.
- **SSH socket/API tunnel:** keep the daemon private and tunnel the socket/API
  from a trusted workstation.
- **HTTPS GUI:** run `patentmine api` behind a reverse proxy with TLS and an API
  token.

Recommended initial VPS posture:

- Bind the HTTP API to localhost unless a reverse proxy is configured.
- Use `PATENTMINE_API_TOKEN` for any remote HTTP API exposure.
- Use TLS at the reverse proxy or `PATENTMINE_API_TLS_CERT` /
  `PATENTMINE_API_TLS_KEY` when serving directly.
- Do not expose the SQLite database or secret directory over a network share.

---

## 6. Credential Status In UI

The daemon should be the source of truth for credential status. TUI/GUI should ask
the daemon for redacted status such as:

```json
{
  "credentials_dir": "/etc/patentmine/secrets",
  "providers": [
    { "name": "uspto", "configured": true, "secret_file": "uspto_odp_key" },
    { "name": "epo_ops", "configured": false, "secret_file": "epo_ops_consumer_key" }
  ]
}
```

The status may show paths, basenames, and health results, but must not return raw
secret values.

---

## 7. Deployment Checklist

1. Create the `patentmine` user on the VPS.
2. Create `/etc/patentmine/secrets` and `/var/lib/patentmine` with restrictive
   ownership and permissions.
3. Copy runtime secret files from the build-host mirror into
   `/etc/patentmine/secrets`.
4. Install `/etc/patentmine/patentmine.env` using `file:` references.
5. Start `patentmine serve` under systemd or another process supervisor.
6. Verify with `patentmine paths` and provider-specific checks.
7. Connect with TUI over SSH or expose the GUI through a secured API/reverse proxy.

---

## 8. Rotation

Rotate secrets by replacing the file in the build-host mirror, syncing it to the
deploy secret directory, then restarting or reloading the daemon.

Do not edit raw secret values into shell history. Prefer file writes from a secure
password manager or a local protected file operation, then set mode `0600`.
