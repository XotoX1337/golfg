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
// moves to DRAWN (closed for joining). Stale OPEN rounds time out to EXPIRED via
// ExpireStale; the host can ReRoll the teams of a DRAWN round.
package session

import (
	"errors"
	"sort"
	"strings"
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
	ErrNotCreator      = errors.New("only the host may do this")
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
	EntraOID    string // Entra object id from SSO; "" for dev-login users (no SSO)
	Team        string // "" | "A" | "B" | ...
	Elo         int    // current rating, used to seed the team-vs-team ELO update
}

// Team is a drawn team with its members, for display.
type Team struct {
	Label   string
	Members []Participant
}

// Name is the team's display name, built from its members' first names joined
// with " & " (e.g. "Anton & Berta"). The internal Label ("A"/"B") stays the
// stable identifier (winner value, comparisons); Name is purely for display. It
// falls back to "Team <Label>" only when no member name is known, so a header is
// never empty.
func (t Team) Name() string {
	names := make([]string, 0, len(t.Members))
	for _, m := range t.Members {
		if fn := firstName(m.DisplayName); fn != "" {
			names = append(names, fn)
		}
	}
	if len(names) == 0 {
		return "Team " + t.Label
	}
	return strings.Join(names, " & ")
}

// firstName returns the first whitespace-separated token of a display name
// ("Anton Müller" -> "Anton"), or "" when the name is empty.
func firstName(displayName string) string {
	if f := strings.Fields(displayName); len(f) > 0 {
		return f[0]
	}
	return ""
}

// TeamName returns the display Name of the team carrying label, or the label
// itself when no team matches (e.g. a tie, where there is no winning team).
func TeamName(teams []Team, label string) string {
	for _, t := range teams {
		if t.Label == label {
			return t.Name()
		}
	}
	return label
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

// HistoryEntry is one finished match for the history view: the session with its
// activity, the teams as drawn and the winning team's label.
type HistoryEntry struct {
	Session    *Session
	Activity   *activity.Activity
	Teams      []Team
	WinnerTeam string
}

// Winner returns the display Name of the winning team, or "" for a tie (no
// winner recorded), so the template can omit the winner line.
func (e HistoryEntry) Winner() string {
	if e.WinnerTeam == "" {
		return ""
	}
	return TeamName(e.Teams, e.WinnerTeam)
}

// Stat is one player's tally across finished matches (a leaderboard row).
type Stat struct {
	DisplayName string
	Elo         int
	Played      int
	Wins        int
}

// History is the view model for the history/stats page: recent finished matches,
// a per-player leaderboard and the total number of matches played.
type History struct {
	Entries []HistoryEntry
	Stats   []Stat
	Total   int
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
