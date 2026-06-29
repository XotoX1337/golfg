# syntax=docker/dockerfile:1

# ---- build stage -------------------------------------------------------------
FROM golang:1.25-alpine AS build
WORKDIR /src

# Resolve modules first so they stay cached across source-only changes.
COPY go.mod go.sum ./
RUN go mod download

# modernc.org/sqlite is pure Go, so CGO stays off and the result is a fully
# static binary (templates and static assets are embedded via //go:embed).
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/golfg ./cmd/golfg

# ---- runtime stage -----------------------------------------------------------
FROM alpine:3.20

# su-exec lets the entrypoint fix volume ownership as root, then drop to an
# unprivileged user before exec'ing the app. wget (busybox) backs the healthcheck.
RUN apk add --no-cache su-exec \
    && adduser -D -H -u 10001 golfg

# The canonical binary and example config live in the image; the actual data
# (config, log, SQLite DB) lives in the /data volume — see entrypoint.sh.
COPY --from=build /out/golfg /usr/local/bin/golfg
COPY golfg.example.toml /usr/local/share/golfg/golfg.example.toml
COPY docs/docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENV GOLFG_APP_HOST=0.0.0.0 \
    GOLFG_APP_PORT=9000
EXPOSE 9000
VOLUME /data
WORKDIR /data

# golfg keeps config/log/db next to its own binary, so the entrypoint runs it
# from /data. The container starts as root only to chown the volume, then drops.
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
