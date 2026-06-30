// Package user models the people using the app and caches their identity coming
// from SSO (or the local dev login). Users are stored in the SQLite `users`
// table; SSO users are keyed by their Entra object id, dev users have a NULL
// entra_oid.
package user

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/XotoX1337/golfg/internal/store"
)

// User is a cached identity. EntraOID is empty for dev-mode users.
type User struct {
	ID          string
	EntraOID    string
	DisplayName string
	Email       string
	HasPhoto    bool // a cached M365 profile photo exists (served via /avatar/:id)
}

// Repository provides access to the users table.
type Repository struct {
	db *sql.DB
}

// NewRepository wires a Repository to the store's database handle.
func NewRepository(st *store.Store) *Repository {
	return &Repository{db: st.DB}
}

// selectColumns deliberately probes photo presence with EXISTS rather than
// joining user_photos — every user scan stays cheap and never reads the image
// bytes (those are streamed on demand by the /avatar route).
const selectColumns = `SELECT id, COALESCE(entra_oid, ''), display_name, COALESCE(email, ''),
	EXISTS(SELECT 1 FROM user_photos WHERE user_photos.user_id = users.id) FROM users`

// GetByID returns the user with the given internal id, or nil if none exists.
func (r *Repository) GetByID(id string) (*User, error) {
	return r.scanOne(selectColumns+` WHERE id = ?`, id)
}

// UpsertByEntraOID caches an SSO identity: it inserts a new user for an unseen
// Entra object id, or refreshes the display name/email of an existing one.
func (r *Repository) UpsertByEntraOID(entraOID, displayName, email string) (*User, error) {
	if entraOID == "" {
		return nil, fmt.Errorf("entra oid required")
	}
	_, err := r.db.Exec(`
		INSERT INTO users (id, entra_oid, display_name, email)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(entra_oid) DO UPDATE SET
			display_name = excluded.display_name,
			email        = excluded.email`,
		uuid.NewString(), entraOID, displayName, email)
	if err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
	}
	return r.scanOne(selectColumns+` WHERE entra_oid = ?`, entraOID)
}

// UpsertDev caches a dev-login identity. Dev users carry a NULL entra_oid and are
// matched by display name so repeated logins with the same name reuse one record.
func (r *Repository) UpsertDev(displayName, email string) (*User, error) {
	if displayName == "" {
		return nil, fmt.Errorf("display name required")
	}
	var id string
	err := r.db.QueryRow(
		`SELECT id FROM users WHERE entra_oid IS NULL AND display_name = ?`, displayName,
	).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = uuid.NewString()
		if _, err := r.db.Exec(
			`INSERT INTO users (id, entra_oid, display_name, email) VALUES (?, NULL, ?, ?)`,
			id, displayName, email,
		); err != nil {
			return nil, fmt.Errorf("insert dev user: %w", err)
		}
	case err != nil:
		return nil, fmt.Errorf("lookup dev user: %w", err)
	default:
		if _, err := r.db.Exec(`UPDATE users SET email = ? WHERE id = ?`, email, id); err != nil {
			return nil, fmt.Errorf("update dev user: %w", err)
		}
	}
	return r.GetByID(id)
}

// scanOne runs a single-row query and returns nil (no error) when there is no row.
func (r *Repository) scanOne(query string, args ...any) (*User, error) {
	var u User
	err := r.db.QueryRow(query, args...).Scan(&u.ID, &u.EntraOID, &u.DisplayName, &u.Email, &u.HasPhoto)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
