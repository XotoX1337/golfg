-- Initial schema for go LFG (domain model: Activity, User, Session, Participation).

CREATE TABLE users (
    id           TEXT PRIMARY KEY,           -- internal id (e.g. uuid)
    entra_oid    TEXT UNIQUE,                -- Entra object id from SSO (nullable in dev)
    display_name TEXT NOT NULL,
    email        TEXT
);

CREATE TABLE activities (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    name             TEXT NOT NULL UNIQUE,
    required_players INTEGER NOT NULL,
    team_size        INTEGER NOT NULL,
    draw_strategy    TEXT NOT NULL           -- e.g. 'random'
);

CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    activity_id INTEGER NOT NULL REFERENCES activities(id),
    creator_id  TEXT NOT NULL REFERENCES users(id),
    status      TEXT NOT NULL,               -- OPEN | FULL | DRAWN | DONE | CANCELLED | EXPIRED
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at  TIMESTAMP
);

CREATE INDEX idx_sessions_status ON sessions(status);

CREATE TABLE participations (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id),
    joined_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    team       TEXT,                         -- 'A' | 'B' after the draw, NULL before
    PRIMARY KEY (session_id, user_id)
);
