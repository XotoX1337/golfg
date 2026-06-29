# syntax=docker/dockerfile:1

# Runtime-only image: it ships the published release binary instead of compiling
# from source. Pick the version with --build-arg GOLFG_VERSION=vX.Y.Z (default
# below). The released binary is the fully static, UPX-compressed Linux build;
# templates and static assets are embedded via //go:embed.
FROM alpine:3.20

ARG GOLFG_VERSION=v1.0.0
ARG GOLFG_REPO=XotoX1337/golfg

# su-exec lets the entrypoint fix volume ownership as root, then drop to an
# unprivileged user before exec'ing the app. wget (busybox) backs the healthcheck.
# ca-certificates is needed for the TLS calls golfg makes (Entra/OIDC, Teams).
RUN apk add --no-cache su-exec ca-certificates \
    && adduser -D -H -u 10001 golfg

# The canonical binary and example config live in the image; the actual data
# (config, log, SQLite DB) lives in the /data volume — see entrypoint.sh.
ADD --chmod=0755 https://github.com/${GOLFG_REPO}/releases/download/${GOLFG_VERSION}/golfg-linux-amd64 /usr/local/bin/golfg
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
