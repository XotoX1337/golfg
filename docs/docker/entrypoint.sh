#!/bin/sh
# Entrypoint for the golfg container.
#
# golfg keeps its config, log and SQLite DB *next to its own binary*. To keep
# that data on a single persisted volume while still allowing image upgrades,
# we run the binary from /data and refresh it from the image on every start.
set -eu

DATA_DIR=/data

# Refresh the binary from the image each start: pulling a newer image upgrades
# the app, while everything else under /data (config, log, db) is preserved.
cp /usr/local/bin/golfg "$DATA_DIR/golfg"

# Seed an editable config on first run. ENV (GOLFG_*) still overrides the file.
if [ ! -f "$DATA_DIR/golfg.toml" ]; then
    cp /usr/local/share/golfg/golfg.example.toml "$DATA_DIR/golfg.toml"
fi

# A freshly created volume is root-owned; hand it to the unprivileged user.
chown -R golfg:golfg "$DATA_DIR"

exec su-exec golfg:golfg "$DATA_DIR/golfg" "$@"
