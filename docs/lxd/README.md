# LXD deployment

LXD runs golfg in a lightweight **system container** (a full Ubuntu userspace
with its own `systemd`), which suits golfg's "plain foreground process managed
by an init system" model. You ship the published Linux binary, run it as a
systemd service inside the container, and expose the port to the host with a
proxy device.

This mirrors the [systemd setup in the main README](../../README.md#as-a-systemd-service),
just inside a container.

## 1. Get the Linux binary

Download the latest released binary instead of building from source:

```bash
mkdir -p dist
curl -fsSL -o dist/golfg-linux-amd64 \
  https://github.com/XotoX1337/golfg/releases/latest/download/golfg-linux-amd64
```

The released binary is UPX-compressed and fully static — it runs as-is on the
Ubuntu container. To pin a specific version, swap `latest` for a tag, e.g.
`.../releases/download/v1.0.0/golfg-linux-amd64`.

> The release only publishes **amd64**. If the LXD host is ARM (e.g. a Raspberry
> Pi), build it yourself with `make compile` and push `dist/golfg-linux-arm`.

## 2. Launch a container

```bash
lxc launch ubuntu:24.04 golfg
```

## 3. Push the binary and config

golfg keeps its config, log and SQLite DB **next to the binary**, so everything
lives in one directory inside the container:

```bash
lxc exec golfg -- mkdir -p /opt/golfg

# Binary
lxc file push dist/golfg-linux-amd64 golfg/opt/golfg/golfg --mode 0755

# Config (edit it afterwards, or rely on ENV — see step 4)
lxc file push golfg.example.toml golfg/opt/golfg/golfg.toml --mode 0644
```

Create an unprivileged service user inside the container:

```bash
lxc exec golfg -- useradd --system --no-create-home --shell /usr/sbin/nologin golfg
lxc exec golfg -- chown -R golfg:golfg /opt/golfg
```

## 4. (Optional) Secrets via an env file

Keep secrets out of `golfg.toml` — `GOLFG_*` env vars override the file:

```bash
cat > golfg.env <<'EOF'
GOLFG_APP_BASE_URL=https://kicker.intranet
GOLFG_SESSION_COOKIE_SECURE=true
GOLFG_AUTH_TENANT_ID=...
GOLFG_AUTH_CLIENT_ID=...
GOLFG_AUTH_CLIENT_SECRET=...
GOLFG_TEAMS_WEBHOOK_URL=...
EOF

lxc file push golfg.env golfg/etc/golfg.env --mode 0600
rm golfg.env        # don't leave secrets on the host
```

Leave the auth/Teams values empty to run in dev mode (no SSO, Teams posts only
logged).

## 5. Install the systemd unit

```bash
cat > golfg.service <<'EOF'
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

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/golfg

[Install]
WantedBy=multi-user.target
EOF

lxc file push golfg.service golfg/etc/systemd/system/golfg.service --mode 0644
rm golfg.service

lxc exec golfg -- systemctl daemon-reload
lxc exec golfg -- systemctl enable --now golfg
lxc exec golfg -- systemctl status golfg --no-pager
```

`Type=simple` fits because golfg stays in the foreground; systemd sends
`SIGTERM` on stop, which triggers the graceful shutdown. `ReadWritePaths=/opt/golfg`
lets the SQLite DB and log file be written under `ProtectSystem=strict`.

## 6. Expose the port to the host

By default the app is only reachable on the container's own IP
(`lxc list golfg`). To publish it on a host port, add a proxy device:

```bash
# host 9000  ->  container 9000
lxc config device add golfg http proxy \
  listen=tcp:0.0.0.0:9000 connect=tcp:127.0.0.1:9000
```

Now the app answers on `http://<host>:9000`. Front it with a reverse proxy for
TLS and set `GOLFG_APP_BASE_URL` / `GOLFG_SESSION_COOKIE_SECURE=true` accordingly.

## Operating

```bash
lxc exec golfg -- journalctl -u golfg -f      # follow logs
lxc exec golfg -- systemctl restart golfg     # restart

# Upgrade: push a new binary and restart
lxc file push dist/golfg-linux-amd64 golfg/opt/golfg/golfg --mode 0755
lxc exec golfg -- chown golfg:golfg /opt/golfg/golfg
lxc exec golfg -- systemctl restart golfg

# Back up the database
lxc file pull golfg/opt/golfg/golfg.db ./golfg.db.bak
```

The data (`golfg.db`) lives inside the container's `/opt/golfg`; snapshot the
whole container with `lxc snapshot golfg` before risky changes.
