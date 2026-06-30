# golfg

**go LFG** ("GoLang Looking For Group Tool") — a small Go web app for spontaneously finding
colleagues to play **foosball** at the office. One person starts a session, others join live,
and once there are four players the app draws the 2v2 teams and announces them in Microsoft Teams.

It's a single self-contained binary (HTML templates and static assets are embedded), served by
[Fiber v2](https://github.com/gofiber/fiber) and made interactive with [htmx](https://htmx.org).

## How it works

1. Open the app on the intranet → you're recognized via **Entra SSO** (name/email from M365).
2. Start a **session** ("I want to play") → the app posts to the Teams channel with a deep link.
3. Colleagues open the app, see the open session **live**, and join (or leave again before it fills).
4. At **4 players** the app **draws 2v2** and posts the teams to Teams. Not happy with the draw?
   The host can **re-roll**.
5. After the match, a player records **who won** — results feed a per-player **leaderboard** on the
   history page.

Open sessions that never fill **auto-expire** after a configurable timeout. Teams is purely a
notification channel — all interaction happens in the app. The app also runs **without** Entra/Teams
configured (graceful degradation: Teams posts are just logged, auth is optional in dev mode), so it
can be tried out standalone.

The UI is mobile-first, **light/dark**, available in **English and German** (auto-detected, with a
switcher), and **white-label** — app name, accent color and the "I want to play" wording (button
label and Teams headline) are configurable so anyone can re-brand it.

## Tech stack

| Area | Choice |
|---|---|
| Language / web | Go + Fiber v2 |
| UI | htmx + Go templates (live updates via polling) |
| i18n | go-i18n (English + German, embedded TOML catalogs) |
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

[branding]        # white-label the public app (no secrets)
app_name = ""                            # empty = "go LFG"
accent_color = ""                        # any CSS color; empty = default teal
play_cta = ""                            # custom "I want to play" button label; empty = localized default
play_announcement = ""                   # custom Teams "session started" headline; Go text/template, {{.Name}} = creator, e.g. "{{.Name}} wants to play!"; empty = localized default

[auth]            # leave empty = dev mode without SSO
tenant_id = ""
client_id = ""
client_secret = ""                       # prefer ENV: GOLFG_AUTH_CLIENT_SECRET

[teams]           # leave empty = posts are only logged
webhook_url = ""                         # prefer ENV: GOLFG_TEAMS_WEBHOOK_URL
lang = "en"                              # channel notification language: "en" or "de"

[session]
expire_minutes = 30                      # 0 = never auto-expire
cookie_secure = false                    # set true in production behind HTTPS
```

The binary keeps its config, log and database files (`golfg.toml`, `golfg.log`, `golfg.db`)
**next to the executable**. Override the port at runtime with `--port`/`-p` or `GOLFG_APP_PORT`.

### Naming

`golfg` is the **project name** (Go + LFG) — it's only the repo, module path and `GOLFG_*` config
prefix. The running app shows whatever you set as `[branding].app_name`, and in day-to-day use it's
really called after the host it lives on. Point it at `kicker.intra.net`, set `app_name = "Kicker"`,
and "hey, let's play!" is all your colleagues need — no global product name required. White-label
it freely.

## Deployment

golfg is built to run in a **container** (Docker / Linux LXD). It's a plain foreground HTTP
server that shuts down gracefully on `SIGINT`/`SIGTERM`, so the container runtime or an init
system manages its lifecycle. Pick a guide:

- **[Docker / docker compose](docs/docker/)** — `docker compose up -d --build`; the image is
  built from this repo and data is persisted on a volume.
- **[LXD system container](docs/lxd/)** — ship the cross-compiled binary into an Ubuntu
  container and run it as a systemd service.

The bare-metal systemd setup below is the same pattern the LXD guide uses, without a container.

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
