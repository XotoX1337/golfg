package session

import (
	"github.com/XotoX1337/golfg/internal/activity"
	"go.uber.org/zap"
)

// SessionStartedEvent describes a freshly opened session worth announcing: a
// player started a round and is waiting for others to join.
type SessionStartedEvent struct {
	Session   *Session
	Activity  *activity.Activity
	Creator   Participant // the user who started it (already joined)
	Count     int         // current participant count (1 right after start)
	Required  int         // players needed before the draw triggers
	FreeSlots int         // Required - Count, i.e. open seats
}

// TeamsDrawnEvent describes a session that just reached its required player
// count and had its teams drawn.
type TeamsDrawnEvent struct {
	Session  *Session
	Activity *activity.Activity
	Teams    []Team // the drawn teams, in label order
}

// MatchResult is the payload describing a finished match, handed to the Notifier.
type MatchResult struct {
	Session    *Session
	Activity   *activity.Activity
	Teams      []Team // the drawn teams, in label order
	WinnerTeam string // label of the winning team ("A", "B", ...)
	FinishedBy string // user id of the participant who ended the match
}

// Notifier receives session lifecycle events worth broadcasting (e.g. to a Teams
// channel). The Manager calls these from inside its locked critical section, so
// implementations must not block — the real Teams client posts asynchronously.
//
// Posts are kept sparse on purpose (see docs/PLAN.md §5): only SessionStarted
// and TeamsDrawn fan out to Teams; joins/leaves are visible live in the app and
// MatchFinished is logged only.
type Notifier interface {
	SessionStarted(SessionStartedEvent)
	TeamsDrawn(TeamsDrawnEvent)
	MatchFinished(MatchResult)
}

// logNotifier is the default Notifier: it logs events instead of posting them.
// It is also what the app falls back to when no Teams webhook is configured,
// mirroring the "no webhook → only log" graceful-degradation contract.
type logNotifier struct{ logger *zap.Logger }

func (n logNotifier) SessionStarted(e SessionStartedEvent) {
	n.logger.Info("session started (notify)",
		zap.String("session", e.Session.ID),
		zap.String("creator", e.Creator.DisplayName),
		zap.Int("free", e.FreeSlots),
	)
}

func (n logNotifier) TeamsDrawn(e TeamsDrawnEvent) {
	n.logger.Info("teams drawn (notify)",
		zap.String("session", e.Session.ID),
		zap.Int("teams", len(e.Teams)),
	)
}

func (n logNotifier) MatchFinished(r MatchResult) {
	n.logger.Info("match finished (notify)",
		zap.String("session", r.Session.ID),
		zap.String("activity", r.Activity.Name),
		zap.String("winner", r.WinnerTeam),
		zap.String("finished_by", r.FinishedBy),
	)
}
