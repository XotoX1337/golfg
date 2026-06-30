package session

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/XotoX1337/golfg/internal/store"
)

// Repository is the SQLite persistence for sessions and participations.
type Repository struct {
	db *sql.DB
}

// NewRepository wires a Repository to the store's database handle.
func NewRepository(st *store.Store) *Repository {
	return &Repository{db: st.DB}
}

const sessionColumns = `SELECT id, activity_id, creator_id, status, created_at, expires_at, winner_team FROM sessions`

// Create inserts a new session row. created_at is left to the DB default.
func (r *Repository) Create(s *Session) error {
	var expires any
	if !s.ExpiresAt.IsZero() {
		expires = s.ExpiresAt.UTC()
	}
	_, err := r.db.Exec(
		`INSERT INTO sessions (id, activity_id, creator_id, status, expires_at) VALUES (?, ?, ?, ?, ?)`,
		s.ID, s.ActivityID, s.CreatorID, string(s.Status), expires,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// Active returns the current session (status OPEN or DRAWN), most recent first,
// or nil when there is none. Ordering by rowid gives reliable insertion order.
func (r *Repository) Active() (*Session, error) {
	return r.scanOne(sessionColumns + ` WHERE status IN ('OPEN','DRAWN') ORDER BY rowid DESC LIMIT 1`)
}

// Get returns the session with the given id, or nil if none exists.
func (r *Repository) Get(id string) (*Session, error) {
	return r.scanOne(sessionColumns+` WHERE id = ?`, id)
}

// FinishedSessions returns the most recently finished (DONE) sessions, newest
// first, capped at limit. These are the matches the history page lists.
func (r *Repository) FinishedSessions(limit int) ([]*Session, error) {
	rows, err := r.db.Query(sessionColumns+` WHERE status = 'DONE' ORDER BY rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list finished sessions: %w", err)
	}
	defer rows.Close()

	var out []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CountFinished returns the total number of finished (DONE) matches ever played.
func (r *Repository) CountFinished() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(1) FROM sessions WHERE status = 'DONE'`).Scan(&n)
	return n, err
}

// Leaderboard tallies every player's finished matches and wins, ranked best
// first. See leaderboard for the ranking criteria.
func (r *Repository) Leaderboard() ([]Stat, error) {
	return r.leaderboard(-1)
}

// TopPlayers returns the highest-ranked players, capped at limit, using the same
// ranking as Leaderboard (a limit of -1 means no cap).
func (r *Repository) TopPlayers(limit int) ([]Stat, error) {
	return r.leaderboard(limit)
}

// leaderboard tallies each player's finished matches and wins (their team equals
// the session's winning team), ranked by ELO rating (the primary criterion),
// then wins and matches played, capped at limit (-1 = all). One small grouped
// query — the leaderboard is read-rarely and the data set is tiny.
func (r *Repository) leaderboard(limit int) ([]Stat, error) {
	rows, err := r.db.Query(`
		SELECT u.display_name,
		       u.elo,
		       COUNT(*) AS played,
		       COALESCE(SUM(CASE WHEN p.team = s.winner_team THEN 1 ELSE 0 END), 0) AS wins
		FROM participations p
		JOIN sessions s ON s.id = p.session_id
		JOIN users u ON u.id = p.user_id
		WHERE s.status = 'DONE'
		GROUP BY p.user_id
		ORDER BY u.elo DESC, wins DESC, played DESC, u.display_name
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("leaderboard: %w", err)
	}
	defer rows.Close()

	var out []Stat
	for rows.Next() {
		var st Stat
		if err := rows.Scan(&st.DisplayName, &st.Elo, &st.Played, &st.Wins); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ExpireStale flips OPEN sessions whose expiry has passed to EXPIRED and returns
// the affected session ids. DRAWN sessions (a match in progress) are never
// expired — only rounds still gathering players time out.
func (r *Repository) ExpireStale(now time.Time) ([]string, error) {
	rows, err := r.db.Query(
		`SELECT id FROM sessions WHERE status = 'OPEN' AND expires_at IS NOT NULL AND expires_at <= ?`,
		now.UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("find stale sessions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, id := range ids {
		if _, err := r.db.Exec(`UPDATE sessions SET status = ? WHERE id = ?`, string(StatusExpired), id); err != nil {
			return nil, fmt.Errorf("expire session %s: %w", id, err)
		}
	}
	return ids, nil
}

// SetStatus updates a session's lifecycle state.
func (r *Repository) SetStatus(id string, st Status) error {
	_, err := r.db.Exec(`UPDATE sessions SET status = ? WHERE id = ?`, string(st), id)
	if err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	return nil
}

// AddParticipant adds a user to a session (before the draw, so team is NULL).
func (r *Repository) AddParticipant(sessionID, userID string) error {
	_, err := r.db.Exec(
		`INSERT INTO participations (session_id, user_id) VALUES (?, ?)`,
		sessionID, userID,
	)
	if err != nil {
		return fmt.Errorf("add participant: %w", err)
	}
	return nil
}

// RemoveParticipant drops a user from a session.
func (r *Repository) RemoveParticipant(sessionID, userID string) error {
	_, err := r.db.Exec(
		`DELETE FROM participations WHERE session_id = ? AND user_id = ?`,
		sessionID, userID,
	)
	if err != nil {
		return fmt.Errorf("remove participant: %w", err)
	}
	return nil
}

// IsParticipant reports whether the user is already in the session.
func (r *Repository) IsParticipant(sessionID, userID string) (bool, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(1) FROM participations WHERE session_id = ? AND user_id = ?`,
		sessionID, userID,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CountParticipants returns how many users are in the session.
func (r *Repository) CountParticipants(sessionID string) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(1) FROM participations WHERE session_id = ?`, sessionID,
	).Scan(&n)
	return n, err
}

// Participants returns the session's participants joined with their user record,
// ordered by join time.
func (r *Repository) Participants(sessionID string) ([]Participant, error) {
	rows, err := r.db.Query(`
		SELECT p.user_id, COALESCE(u.display_name, ''), COALESCE(u.email, ''), COALESCE(u.entra_oid, ''), COALESCE(p.team, ''), u.elo
		FROM participations p
		JOIN users u ON u.id = p.user_id
		WHERE p.session_id = ?
		ORDER BY p.joined_at, p.rowid`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list participants: %w", err)
	}
	defer rows.Close()

	var out []Participant
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.UserID, &p.DisplayName, &p.Email, &p.EntraOID, &p.Team, &p.Elo); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ApplyDraw persists a team assignment and flips the session to DRAWN, atomically.
// teams[i] holds the user ids of team i, labelled A, B, ... by index.
func (r *Repository) ApplyDraw(sessionID string, teams [][]string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	for i, members := range teams {
		label := teamLabel(i)
		for _, userID := range members {
			if _, err := tx.Exec(
				`UPDATE participations SET team = ? WHERE session_id = ? AND user_id = ?`,
				label, sessionID, userID,
			); err != nil {
				return fmt.Errorf("assign team: %w", err)
			}
		}
	}
	if _, err := tx.Exec(
		`UPDATE sessions SET status = ? WHERE id = ?`, string(StatusDrawn), sessionID,
	); err != nil {
		return fmt.Errorf("mark drawn: %w", err)
	}
	return tx.Commit()
}

// Finish records the winning team, closes the session as DONE and applies the
// per-player ELO deltas — all in one transaction, so the rating update never
// drifts out of step with the match result. eloDeltas maps each participant's
// user id to the rating change to add; a nil/empty map records the result
// without touching any rating.
func (r *Repository) Finish(sessionID, winnerTeam string, eloDeltas map[string]int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	if _, err := tx.Exec(
		`UPDATE sessions SET winner_team = ?, status = ? WHERE id = ?`,
		winnerTeam, string(StatusDone), sessionID,
	); err != nil {
		return fmt.Errorf("finish session: %w", err)
	}
	for userID, delta := range eloDeltas {
		if _, err := tx.Exec(
			`UPDATE users SET elo = elo + ? WHERE id = ?`, delta, userID,
		); err != nil {
			return fmt.Errorf("apply elo to %s: %w", userID, err)
		}
	}
	return tx.Commit()
}

// scanOne runs a single-row session query, returning nil (no error) for no row.
func (r *Repository) scanOne(query string, args ...any) (*Session, error) {
	s, err := scanSession(r.db.QueryRow(query, args...))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so a single session
// row can be decoded from either a single-row query or a result set.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanSession decodes one session row (in sessionColumns order), normalizing the
// nullable expires_at and winner_team columns.
func scanSession(sc rowScanner) (*Session, error) {
	var s Session
	var status string
	var expires sql.NullTime
	var winner sql.NullString
	if err := sc.Scan(&s.ID, &s.ActivityID, &s.CreatorID, &status, &s.CreatedAt, &expires, &winner); err != nil {
		return nil, err
	}
	s.Status = Status(status)
	if expires.Valid {
		s.ExpiresAt = expires.Time
	}
	s.WinnerTeam = winner.String
	return &s, nil
}
