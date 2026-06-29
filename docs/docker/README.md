# Docker deployment

Run golfg as a container. The image is a small Alpine runtime wrapping the
**published release binary** (fully static, UPX-compressed, with templates and
static assets embedded) — it does **not** compile from source. Config, log and
the SQLite database are persisted on a named volume.

Files in this directory:

| File | Purpose |
|---|---|
| `Dockerfile` | Alpine runtime that pulls the release binary (`ADD` from GitHub). Override the tag with `--build-arg GOLFG_VERSION=vX.Y.Z`. |
| `docker-compose.yml` | One-command build + run with a persisted `golfg-data` volume. |
| `entrypoint.sh` | Seeds config on first run, refreshes the binary, drops to an unprivileged user. |
| `.env.example` | Template for secrets (auth / Teams). Copy to `.env`. |

> The release ships a **linux/amd64** binary, so the image is amd64-only. For
> ARM (e.g. a Raspberry Pi) use the [LXD guide](../lxd/README.md) or build from
> source.

## Quick start (compose)

From the **repo root**:

```bash
cp docs/docker/.env.example docs/docker/.env   # optional: add auth/Teams secrets
docker compose -f docs/docker/docker-compose.yml up -d --build
```

The app is now on <http://localhost:9000>. Logs and lifecycle:

```bash
docker compose -f docs/docker/docker-compose.yml logs -f
docker compose -f docs/docker/docker-compose.yml down      # stop (keeps the volume)
```

With no secrets set it runs in **dev mode**: no SSO, and Teams posts are only
logged. Fill in `.env` to enable Entra auth and Teams notifications.

## Plain docker (no compose)

```bash
# Build (context = repo root for golfg.example.toml + entrypoint.sh; the binary
# itself is pulled from the release). Override the tag with --build-arg if needed.
docker build -f docs/docker/Dockerfile --build-arg GOLFG_VERSION=v1.0.0 -t golfg:latest .

# Run with a persisted volume
docker volume create golfg-data
docker run -d --name golfg \
  -p 9000:9000 \
  -v golfg-data:/data \
  -e GOLFG_APP_BASE_URL="http://localhost:9000" \
  --env-file docs/docker/.env \
  --restart unless-stopped \
  golfg:latest
```

## Configuration

Everything in `golfg.toml` can be set as a `GOLFG_<SECTION>_<KEY>` environment
variable, and **ENV always overrides the file**, so secrets never have to be
written into the volume. Common ones:

| Variable | Meaning |
|---|---|
| `GOLFG_APP_BASE_URL` | Public URL used for Teams deep links — set to how users reach the app. |
| `GOLFG_APP_PORT` | Listen port inside the container (default `9000`). |
| `GOLFG_SESSION_COOKIE_SECURE` | Set `true` when HTTPS is terminated in front of the container. |
| `GOLFG_AUTH_*` | Entra ID / OIDC credentials (empty ⇒ dev mode). |
| `GOLFG_TEAMS_WEBHOOK_URL` | Teams incoming webhook (empty ⇒ posts only logged). |

To edit the file form instead, it lives at `/data/golfg.toml` in the volume
(seeded from `golfg.example.toml` on first run).

## How the data volume works

golfg writes its config, log and database **next to its own binary**. To keep
that on one persisted volume *and* allow upgrades, `entrypoint.sh` runs the app
from `/data` and copies a fresh binary out of the image on every start. So:

- **Data persists** in the `golfg-data` volume across restarts.
- **Upgrades**: bump `GOLFG_VERSION` (in `docker-compose.yml` or via
  `--build-arg`) and run `up -d --build` — the image pulls the new release
  binary and the entrypoint refreshes it on start; `golfg.db` is untouched.

Back up the volume by copying out `/data/golfg.db` (e.g.
`docker cp golfg:/data/golfg.db ./golfg.db.bak`).

> Put HTTPS in front of the container (reverse proxy / ingress) for production
> and set `GOLFG_SESSION_COOKIE_SECURE=true` and a real `GOLFG_APP_BASE_URL`.
