// Package session is the core of the app: it owns the single active kicker
// session, its participants and the team draw. It exposes a small Manager API
// (Start / Join / Leave / Lobby) over a SQLite-backed Repository, and keeps the
// team-draw logic isolated and seedable so it can be tested deterministically.
//
// Only one session is active at a time (one table). The lifecycle is
//
//	OPEN -> DRAWN -> DONE
//
// with CANCELLED/EXPIRED as terminal off-ramps. When the participant count
// reaches the activity's RequiredPlayers, the teams are drawn and the session
// moves to DRAWN (closed for joining). Auto-expire and re-roll arrive in WP4.
package session

import (
	"errors"
	"sort"
	"time"

	"github.com/XotoX1337/golfg/internal/activity"
)

// Status is the lifecycle state of a session.
type Status string

const (
	StatusOpen      Status = "OPEN"      // gathering players
	StatusDrawn     Status = "DRAWN"     // full, teams assigned
	StatusDone      Status = "DONE"      // finished / superseded
	StatusCancelled Status = "CANCELLED" // abandoned before it filled
	StatusExpired   Status = "EXPIRED"   // timed out (enforced in WP4)
)

// Errors returned by the Manager. They are "soft" by design: handlers treat them
// as expected races (someone else acted first) and simply re-render the lobby.
var (
	ErrSessionActive   = errors.New("a session is already active")
	ErrSessionNotOpen  = errors.New("session is not open")
	ErrAlreadyJoined   = errors.New("already joined")
	ErrSessionFull     = errors.New("session is full")
	ErrSessionNotDrawn = errors.New("session is not drawn")
	ErrNotParticipant  = errors.New("not a participant of this session")
	ErrInvalidWinner   = errors.New("invalid winning team")
)

// Session is a single kicker round.
type Session struct {
	ID         string
	ActivityID int64
	CreatorID  string
	Status     Status
	CreatedAt  time.Time
	ExpiresAt  time.Time // zero when no expiry is set
	WinnerTeam string    // "" until a finished match records its winning team label
}

// Participant is a user taking part in a session, with their drawn team (empty
// before the draw).
type Participant struct {
	UserID      string
	DisplayName string
	Email       string
	Team        string // "" | "A" | "B" | ...
}

// Team is a drawn team with its members, for display.
type Team struct {
	Label   string
	Members []Participant
}

// Lobby is the view model the templates render. It captures the whole live state
// of the lobby for the current user in one struct.
type Lobby struct {
	HasSession   bool
	Session      *Session
	Activity     *activity.Activity
	Participants []Participant
	Count        int
	Required     int
	IsOpen       bool
	IsDrawn      bool
	Teams        []Team // populated when IsDrawn
	Joined       bool   // current user is a participant
	IsCreator    bool   // current user started the session
}

// groupTeams buckets participants by their team label (A, B, ...) in order.
func groupTeams(parts []Participant) []Team {
	byLabel := map[string][]Participant{}
	var labels []string
	for _, p := range parts {
		if p.Team == "" {
			continue
		}
		if _, seen := byLabel[p.Team]; !seen {
			labels = append(labels, p.Team)
		}
		byLabel[p.Team] = append(byLabel[p.Team], p)
	}
	sort.Strings(labels)
	teams := make([]Team, 0, len(labels))
	for _, l := range labels {
		teams = append(teams, Team{Label: l, Members: byLabel[l]})
	}
	return teams
}
