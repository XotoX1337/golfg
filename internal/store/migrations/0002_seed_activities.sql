-- Seed the baseline activity. The Activity abstraction is the "seam" for future
-- games (Darts, MarioKart, ...); for now only Tischfußball exists.
INSERT OR IGNORE INTO activities (name, required_players, team_size, draw_strategy)
VALUES ('Tischfußball', 4, 2, 'random');
