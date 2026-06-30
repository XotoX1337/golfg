-- Add a per-player ELO rating, the primary leaderboard ranking. Existing users
-- inherit the default starting value; the user-insert paths (internal/user,
-- internal/auth) never name `elo`, so the default applies on every new row.
-- Forward-only: historical matches are not recomputed (see session.Manager.Finish).
ALTER TABLE users ADD COLUMN elo INTEGER NOT NULL DEFAULT 1000;
