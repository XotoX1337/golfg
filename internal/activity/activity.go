// Package activity models game definitions. An Activity is the "seam" for future
// games (Darts, MarioKart, ...) — for now only the seeded "Tischfußball" exists.
// The session core reads requiredPlayers/teamSize from here instead of
// hardcoding 4/2, so a new game is just another row in the activities table.
package activity

import (
	"database/sql"

	"github.com/XotoX1337/golfg/internal/store"
)

// Activity is a game definition. RequiredPlayers is the headcount that triggers
// the team draw; TeamSize is how many players each drawn team holds (the number
// of teams is RequiredPlayers / TeamSize). DrawStrategy selects how teams are
// formed (currently only "random").
type Activity struct {
	ID              int64
	Name            string
	RequiredPlayers int
	TeamSize        int
	DrawStrategy    string
}

// Repository provides read access to the activities table.
type Repository struct {
	db *sql.DB
}

// NewRepository wires a Repository to the store's database handle.
func NewRepository(st *store.Store) *Repository {
	return &Repository{db: st.DB}
}

const selectColumns = `SELECT id, name, required_players, team_size, draw_strategy FROM activities`

// GetByID returns the activity with the given id, or nil if none exists.
func (r *Repository) GetByID(id int64) (*Activity, error) {
	return r.scanOne(selectColumns+` WHERE id = ?`, id)
}

// Default returns the baseline activity (the lowest id). With only one seeded
// game this is "Tischfußball"; it is what a freshly-started session uses.
func (r *Repository) Default() (*Activity, error) {
	return r.scanOne(selectColumns + ` ORDER BY id LIMIT 1`)
}

// scanOne runs a single-row query and returns nil (no error) when there is no row.
func (r *Repository) scanOne(query string, args ...any) (*Activity, error) {
	var a Activity
	err := r.db.QueryRow(query, args...).Scan(&a.ID, &a.Name, &a.RequiredPlayers, &a.TeamSize, &a.DrawStrategy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}
