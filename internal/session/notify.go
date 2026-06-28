package session

import (
	"github.com/XotoX1337/golfg/internal/activity"
	"go.uber.org/zap"
)

// MatchResult is the payload describing a finished match, handed to the Notifier.
type MatchResult struct {
	Session    *Session
	Activity   *activity.Activity
	Teams      []Team // the drawn teams, in label order
	WinnerTeam string // label of the winning team ("A", "B", ...)
	FinishedBy string // user id of the participant who ended the match
}

// Notifier receives session lifecycle events worth broadcasting (e.g. to a Teams
// channel). It is the seam WP3 fills with the real Teams/Adaptive-Card client;
// until then logNotifier just logs, mirroring the "no webhook → only log"
// graceful-degradation contract.
type Notifier interface {
	MatchFinished(MatchResult)
}

// logNotifier is the default Notifier: it logs the event instead of posting it.
type logNotifier struct{ logger *zap.Logger }

func (n logNotifier) MatchFinished(r MatchResult) {
	n.logger.Info("match finished",
		zap.String("session", r.Session.ID),
		zap.String("activity", r.Activity.Name),
		zap.String("winner", r.WinnerTeam),
		zap.String("finished_by", r.FinishedBy),
	)
}
