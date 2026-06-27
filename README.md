# golfg

**go LFG** ("GoLang Looking For Group Tool") — a small Go web app for spontaneously finding
colleagues to play **foosball** at the office. One person starts a session, others join live,
and once there are four players the app draws the 2v2 teams and announces them in Microsoft Teams.

It's a single self-contained binary (HTML templates and static assets are embedded), served by
[Fiber v2](https://github.com/gofiber/fiber) and made interactive with [htmx](https://htmx.org).

## How it works

1. Open the app on the intranet → you're recognized via **Entra SSO** (name/email from M365).
2. Start a **session** ("I want to play") → the app posts to the Teams channel with a deep link.
3. Colleagues open the app, see the open session **live**, and join.
4. At **4 players** the app **draws 2v2**, closes the session, and posts the result to Teams.

Teams is purely a notification channel — all interaction happens in the app. The app also runs
**without** Entra/Teams configured (graceful degradation: Teams posts are just logged, auth is
optional in dev mode), so it can be tried out standalone.

## Tech stack

| Area | Choice |
|---|---|
| Language / web | Go + Fiber v2 |
| UI | htmx + Go templates |
| Persistence | SQLite (`modernc.org/sqlite`, pure Go — no CGo) |
| Auth | Entra ID / OIDC |
| Config | viper (TOML file + ENV) |
| Logging | zap |

## Development

```bash
make run       # go run ./cmd/golfg (local dev)
make build     # build dist/golfg for the host platform
make compile   # cross-compile linux-arm/amd64, windows-amd64, darwin-amd64 into dist/
make esbuild   # bundle/minify assets/css/style.css -> web/static/css/app.min.css (needs esbuild on PATH)
make icon      # regenerate web/static/img/favicon.ico from assets/img/favicon.svg
```

Edit CSS sources under `assets/`, then run `make esbuild` — static assets are served from the
**embedded** `web/static`, so asset changes need a rebuild to take effect.

The restructure plan and per-step work packages live under `docs/` (`docs/PLAN.md`,
`docs/workpackages/`).

## Configuration

Configure via a `golfg.toml` file (copy from `golfg.example.toml`) or via `GOLFG_<SECTION>_<KEY>`
environment variables. **ENV wins over the file**, so secrets can be injected without writing a
file. Real config (`golfg.toml`, `.env`, `golfg.db`, `golfg.log`) is gitignored — never commit
secrets.

```toml
[app]
host = "0.0.0.0"
port = 9000
base_url = "https://kicker.intranet"   # for Teams deep links

[auth]            # leave empty = dev mode without SSO
tenant_id = ""
client_id = ""
client_secret = ""                       # prefer ENV: GOLFG_AUTH_CLIENT_SECRET

[teams]           # leave empty = posts are only logged
webhook_url = ""                         # prefer ENV: GOLFG_TEAMS_WEBHOOK_URL

[session]
expire_minutes = 30
```

The binary keeps its config, log and database files (`golfg.toml`, `golfg.log`, `golfg.db`)
**next to the executable**. Override the port at runtime with `--port`/`-p` or `GOLFG_APP_PORT`.

## Deployment

golfg is built to run in a **container** (Docker / Linux LXD). It's a plain foreground HTTP
server that shuts down gracefully on `SIGINT`/`SIGTERM`, so the container runtime or an init
system manages its lifecycle.

### As a systemd service

1. **Place the binary and config.** Everything lives next to the binary:

   ```bash
   sudo install -d /opt/golfg
   sudo install -m 0755 dist/golfg-linux-amd64 /opt/golfg/golfg
   sudo install -m 0644 golfg.example.toml /opt/golfg/golfg.toml   # then edit it
   ```

2. **Create a dedicated user** so the service doesn't run as root:

   ```bash
   sudo useradd --system --no-create-home --shell /usr/sbin/nologin golfg
   sudo chown -R golfg:golfg /opt/golfg
   ```

3. **(Optional) Put secrets in an env file** instead of `golfg.toml`:

   ```bash
   # /etc/golfg.env
   GOLFG_AUTH_CLIENT_SECRET=...
   GOLFG_TEAMS_WEBHOOK_URL=...
   ```

   ```bash
   sudo chmod 0600 /etc/golfg.env
   ```

4. **Create the unit** at `/etc/systemd/system/golfg.service`:

   ```ini
   [Unit]
   Description=go LFG
   After=network-online.target
   Wants=network-online.target

   [Service]
   Type=simple
   User=golfg
   Group=golfg
   WorkingDirectory=/opt/golfg
   ExecStart=/opt/golfg/golfg
   EnvironmentFile=-/etc/golfg.env
   Restart=on-failure
   RestartSec=5

   # Hardening (optional but recommended)
   NoNewPrivileges=true
   ProtectSystem=strict
   ProtectHome=true
   ReadWritePaths=/opt/golfg

   [Install]
   WantedBy=multi-user.target
   ```

   `Type=simple` fits because golfg stays in the foreground; systemd sends `SIGTERM` on stop,
   which triggers the graceful shutdown. `ReadWritePaths=/opt/golfg` lets the embedded SQLite DB
   and log file be written under `ProtectSystem=strict`.

5. **Enable and start it:**

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now golfg
   sudo systemctl status golfg
   journalctl -u golfg -f
   ```

The app listens on `0.0.0.0:9000` by default.
