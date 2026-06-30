-- Cache the user's Microsoft 365 profile photo, pulled once per login from
-- Graph's /me/photo (see internal/auth). Kept in a separate table — not a BLOB
-- column on `users` — so the common user scans (internal/user selectColumns,
-- the leaderboard, participant lists) never drag the image bytes along: those
-- queries only ever probe presence via EXISTS(...). The photo itself is loaded
-- on demand by the /avatar route.
CREATE TABLE user_photos (
    user_id    TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    photo      BLOB NOT NULL,           -- raw image bytes as returned by Graph
    etag       TEXT NOT NULL,           -- content hash; drives the /avatar ETag and skips no-op rewrites
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
