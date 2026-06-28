package session

import (
	"database/sql"
	"fmt"

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
		SELECT p.user_id, COALESCE(u.display_name, ''), COALESCE(u.email, ''), COALESCE(p.team, '')
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
		if err := rows.Scan(&p.UserID, &p.DisplayName, &p.Email, &p.Team); err != nil {
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

// Finish records the winning team and closes the session as DONE, atomically.
func (r *Repository) Finish(sessionID, winnerTeam string) error {
	_, err := r.db.Exec(
		`UPDATE sessions SET winner_team = ?, status = ? WHERE id = ?`,
		winnerTeam, string(StatusDone), sessionID,
	)
	if err != nil {
		return fmt.Errorf("finish session: %w", err)
	}
	return nil
}

// scanOne runs a single-row session query, returning nil (no error) for no row.
func (r *Repository) scanOne(query string, args ...any) (*Session, error) {
	var s Session
	var status string
	var expires sql.NullTime
	var winner sql.NullString
	err := r.db.QueryRow(query, args...).Scan(
		&s.ID, &s.ActivityID, &s.CreatorID, &status, &s.CreatedAt, &expires, &winner,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Status = Status(status)
	if expires.Valid {
		s.ExpiresAt = expires.Time
	}
	s.WinnerTeam = winner.String
	return &s, nil
}
