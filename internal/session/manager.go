package session

import (
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/XotoX1337/golfg/internal/activity"
	"github.com/XotoX1337/golfg/internal/store"
	"go.uber.org/zap"
)

// newID returns a fresh session id.
func newID() string { return uuid.NewString() }

// Manager orchestrates the single active session. All mutating actions are
// serialized by a process-wide mutex: the app runs as one instance over one
// SQLite file, so a mutex is the simplest correct guard against the race where
// two players join at once and both try to trigger the draw.
type Manager struct {
	repo   *Repository
	acts   *activity.Repository
	logger *zap.Logger
	notify Notifier
	expire time.Duration

	mu  sync.Mutex
	rng *rand.Rand
}

// Option customizes a Manager.
type Option func(*Manager)

// WithRand injects a deterministic RNG (used by tests). The Manager's mutex also
// guards the RNG, which is not safe for concurrent use on its own.
func WithRand(r *rand.Rand) Option {
	return func(m *Manager) { m.rng = r }
}

// WithNotifier swaps the default log-only Notifier for another implementation
// (WP3 plugs the real Teams client in here).
func WithNotifier(n Notifier) Option {
	return func(m *Manager) { m.notify = n }
}

// New builds a session Manager. expireMinutes seeds each session's expires_at
// (enforced by ExpireStale, driven by the server's reaper); the default RNG is
// time-seeded.
func New(st *store.Store, logger *zap.Logger, expireMinutes int, opts ...Option) *Manager {
	m := &Manager{
		repo:   NewRepository(st),
		acts:   activity.NewRepository(st),
		logger: logger,
		notify: logNotifier{logger: logger},
		expire: time.Duration(expireMinutes) * time.Minute,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Start opens a new session for the given creator, who automatically joins it.
// It fails with ErrSessionActive while any session is still active (OPEN or
// DRAWN): one table means one round at a time. A DRAWN round must first be ended
// via Finish before a fresh one can begin.
func (m *Manager) Start(creatorID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, err := m.repo.Active()
	if err != nil {
		return nil, err
	}
	if current != nil {
		// Active() only returns OPEN or DRAWN sessions, both of which occupy the
		// table, so any of them blocks starting a new round.
		return nil, ErrSessionActive
	}

	act, err := m.acts.Default()
	if err != nil {
		return nil, err
	}

	s := &Session{
		ID:         newID(),
		ActivityID: act.ID,
		CreatorID:  creatorID,
		Status:     StatusOpen,
	}
	if m.expire > 0 {
		s.ExpiresAt = time.Now().Add(m.expire)
	}
	if err := m.repo.Create(s); err != nil {
		return nil, err
	}
	if err := m.repo.AddParticipant(s.ID, creatorID); err != nil {
		return nil, err
	}
	// A single-seat activity would be "full" immediately; handle it uniformly.
	drawn, err := m.maybeDraw(s, act)
	if err != nil {
		return nil, err
	}
	m.logger.Info("session started", zap.String("session", s.ID), zap.String("creator", creatorID))

	// Announce the open session unless it drew immediately (the draw already
	// fired its own event in maybeDraw). For the normal kicker flow it stays
	// OPEN with one player, so this is the "X wants to play, n spots left" post.
	if !drawn {
		parts, err := m.repo.Participants(s.ID)
		if err != nil {
			return nil, err
		}
		var creator Participant
		for _, p := range parts {
			if p.UserID == creatorID {
				creator = p
				break
			}
		}
		m.notify.SessionStarted(SessionStartedEvent{
			Session:   s,
			Activity:  act,
			Creator:   creator,
			Count:     len(parts),
			Required:  act.RequiredPlayers,
			FreeSlots: act.RequiredPlayers - len(parts),
		})
	}
	return s, nil
}

// Join adds a user to the given OPEN session and triggers the draw once it is
// full. Double-joins and joins to a closed session are rejected.
func (m *Manager) Join(sessionID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := m.repo.Get(sessionID)
	if err != nil {
		return err
	}
	if s == nil || s.Status != StatusOpen {
		return ErrSessionNotOpen
	}

	joined, err := m.repo.IsParticipant(sessionID, userID)
	if err != nil {
		return err
	}
	if joined {
		return ErrAlreadyJoined
	}

	act, err := m.acts.GetByID(s.ActivityID)
	if err != nil {
		return err
	}
	count, err := m.repo.CountParticipants(sessionID)
	if err != nil {
		return err
	}
	if count >= act.RequiredPlayers {
		return ErrSessionFull
	}

	if err := m.repo.AddParticipant(sessionID, userID); err != nil {
		return err
	}
	if _, err := m.maybeDraw(s, act); err != nil {
		return err
	}
	m.logger.Info("user joined session", zap.String("session", sessionID), zap.String("user", userID))
	return nil
}

// Leave removes a user from an OPEN session (the counter drops). When the last
// participant leaves, the session is cancelled.
func (m *Manager) Leave(sessionID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := m.repo.Get(sessionID)
	if err != nil {
		return err
	}
	if s == nil || s.Status != StatusOpen {
		return ErrSessionNotOpen
	}

	if err := m.repo.RemoveParticipant(sessionID, userID); err != nil {
		return err
	}
	count, err := m.repo.CountParticipants(sessionID)
	if err != nil {
		return err
	}
	if count == 0 {
		if err := m.repo.SetStatus(sessionID, StatusCancelled); err != nil {
			return err
		}
		m.logger.Info("session cancelled (empty)", zap.String("session", sessionID))
	}
	m.logger.Info("user left session", zap.String("session", sessionID), zap.String("user", userID))
	return nil
}

// Finish ends a DRAWN session, recording which team won, and frees the table so
// a new round can start. Any participant may end the match (not just the host).
// winnerTeam must be one of the drawn team labels ("A", "B", ...).
func (m *Manager) Finish(sessionID, userID, winnerTeam string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := m.repo.Get(sessionID)
	if err != nil {
		return err
	}
	if s == nil || s.Status != StatusDrawn {
		return ErrSessionNotDrawn
	}

	joined, err := m.repo.IsParticipant(sessionID, userID)
	if err != nil {
		return err
	}
	if !joined {
		return ErrNotParticipant
	}

	parts, err := m.repo.Participants(sessionID)
	if err != nil {
		return err
	}
	teams := groupTeams(parts)
	if !validTeam(teams, winnerTeam) {
		return ErrInvalidWinner
	}

	// Rate the match (forward-only) on the players' pre-match ratings, then apply
	// the result and the deltas atomically. computeEloDeltas only handles the
	// two-team shape the seeded activity produces; anything else is a deliberate
	// no-op so the match still finishes cleanly.
	eloDeltas := computeEloDeltas(teams, winnerTeam)
	if eloDeltas == nil {
		m.logger.Warn("skipping elo update: not a two-team match",
			zap.String("session", sessionID), zap.Int("teams", len(teams)))
	}
	if err := m.repo.Finish(sessionID, winnerTeam, eloDeltas); err != nil {
		return err
	}
	s.Status = StatusDone
	s.WinnerTeam = winnerTeam

	act, err := m.acts.GetByID(s.ActivityID)
	if err != nil {
		return err
	}
	m.logger.Info("match finished", zap.String("session", sessionID), zap.String("winner", winnerTeam))
	m.notify.MatchFinished(MatchResult{
		Session:    s,
		Activity:   act,
		Teams:      teams,
		WinnerTeam: winnerTeam,
		FinishedBy: userID,
	})
	return nil
}

// ReRoll re-draws the teams of a DRAWN session and re-announces them. Only the
// host may re-roll, and only while the round is still DRAWN (before Finish): it
// is a "the draw felt unfair, shuffle again" escape hatch, not a way to change a
// match that already happened.
func (m *Manager) ReRoll(sessionID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := m.repo.Get(sessionID)
	if err != nil {
		return err
	}
	if s == nil || s.Status != StatusDrawn {
		return ErrSessionNotDrawn
	}
	if s.CreatorID != userID {
		return ErrNotCreator
	}

	act, err := m.acts.GetByID(s.ActivityID)
	if err != nil {
		return err
	}
	parts, err := m.repo.Participants(sessionID)
	if err != nil {
		return err
	}

	ids := make([]string, len(parts))
	for i, p := range parts {
		ids[i] = p.UserID
	}
	teams := drawTeams(ids, act.TeamSize, m.rng)
	// ApplyDraw rewrites every participant's team (all are listed) and re-asserts
	// DRAWN, so it doubles as the re-roll persistence step.
	if err := m.repo.ApplyDraw(sessionID, teams); err != nil {
		return err
	}

	drawn, err := m.repo.Participants(sessionID)
	if err != nil {
		return err
	}
	m.logger.Info("teams re-rolled", zap.String("session", sessionID), zap.String("by", userID))
	m.notify.TeamsDrawn(TeamsDrawnEvent{
		Session:  s,
		Activity: act,
		Teams:    groupTeams(drawn),
	})
	return nil
}

// ExpireStale flips OPEN sessions past their expiry to EXPIRED and returns how
// many were closed. Called periodically by the server's reaper so abandoned
// rounds (someone started one and nobody joined) free the table on their own.
func (m *Manager) ExpireStale() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids, err := m.repo.ExpireStale(time.Now())
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		m.logger.Info("session expired", zap.String("session", id))
	}
	return len(ids), nil
}

// History builds the history/stats view model: the most recent finished matches
// (up to limit, with their teams), a per-player leaderboard and the all-time
// match count.
func (m *Manager) History(limit int) (*History, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessions, err := m.repo.FinishedSessions(limit)
	if err != nil {
		return nil, err
	}

	h := &History{}
	for _, s := range sessions {
		act, err := m.acts.GetByID(s.ActivityID)
		if err != nil {
			return nil, err
		}
		parts, err := m.repo.Participants(s.ID)
		if err != nil {
			return nil, err
		}
		h.Entries = append(h.Entries, HistoryEntry{
			Session:    s,
			Activity:   act,
			Teams:      groupTeams(parts),
			WinnerTeam: s.WinnerTeam,
		})
	}

	if h.Stats, err = m.repo.Leaderboard(); err != nil {
		return nil, err
	}
	if h.Total, err = m.repo.CountFinished(); err != nil {
		return nil, err
	}
	return h, nil
}

// validTeam reports whether label matches one of the drawn teams.
func validTeam(teams []Team, label string) bool {
	for _, t := range teams {
		if t.Label == label {
			return true
		}
	}
	return false
}

// Lobby builds the live view model for currentUserID: the active session (if
// any), its participants, and — once full — the drawn teams.
func (m *Manager) Lobby(currentUserID string) (*Lobby, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, err := m.repo.Active()
	if err != nil {
		return nil, err
	}
	lb := &Lobby{}
	if s == nil {
		return lb, nil
	}

	act, err := m.acts.GetByID(s.ActivityID)
	if err != nil {
		return nil, err
	}
	parts, err := m.repo.Participants(s.ID)
	if err != nil {
		return nil, err
	}

	lb.HasSession = true
	lb.Session = s
	lb.Activity = act
	lb.Participants = parts
	lb.Count = len(parts)
	lb.Required = act.RequiredPlayers
	lb.IsOpen = s.Status == StatusOpen
	lb.IsDrawn = s.Status == StatusDrawn
	lb.IsCreator = s.CreatorID == currentUserID
	for _, p := range parts {
		if p.UserID == currentUserID {
			lb.Joined = true
			break
		}
	}
	if lb.IsDrawn {
		lb.Teams = groupTeams(parts)
	}
	return lb, nil
}

// maybeDraw runs the team draw and flips the session to DRAWN once the required
// player count is reached, returning whether a draw happened. On a successful
// draw it fires the TeamsDrawn notification (it is the single place the draw
// occurs, reached from both Start and Join). Callers must hold m.mu (so the RNG
// is used safely and the count→draw step is atomic against concurrent joins).
func (m *Manager) maybeDraw(s *Session, act *activity.Activity) (bool, error) {
	parts, err := m.repo.Participants(s.ID)
	if err != nil {
		return false, err
	}
	if len(parts) < act.RequiredPlayers {
		return false, nil
	}

	ids := make([]string, len(parts))
	for i, p := range parts {
		ids[i] = p.UserID
	}
	teams := drawTeams(ids, act.TeamSize, m.rng)
	if err := m.repo.ApplyDraw(s.ID, teams); err != nil {
		return false, err
	}
	s.Status = StatusDrawn
	m.logger.Info("teams drawn", zap.String("session", s.ID), zap.Int("teams", len(teams)))

	// Re-read participants so the event carries their drawn team labels and
	// display names (for "Team A: … vs Team B: …").
	drawn, err := m.repo.Participants(s.ID)
	if err != nil {
		return true, err
	}
	m.notify.TeamsDrawn(TeamsDrawnEvent{
		Session:  s,
		Activity: act,
		Teams:    groupTeams(drawn),
	})
	return true, nil
}
