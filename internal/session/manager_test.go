package session

import (
	"errors"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/XotoX1337/golfg/internal/store"
	"github.com/XotoX1337/golfg/internal/user"
)

// newTestEnv spins up a Manager over a fresh on-disk SQLite store (migrations +
// seed applied) with a deterministically seeded RNG, plus a user repository to
// create the people the tests need.
func newTestEnv(t *testing.T) (*Manager, *user.Repository) {
	t.Helper()
	logger := zap.NewNop()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"), logger)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	mgr := New(st, logger, 30, WithRand(rand.New(rand.NewSource(1))))
	return mgr, user.NewRepository(st)
}

func mkUser(t *testing.T, repo *user.Repository, name string) string {
	t.Helper()
	u, err := repo.UpsertDev(name, "")
	if err != nil {
		t.Fatalf("create user %q: %v", name, err)
	}
	return u.ID
}

func TestStartCreatesOpenSessionWithCreatorJoined(t *testing.T) {
	mgr, users := newTestEnv(t)
	anton := mkUser(t, users, "Anton")

	if _, err := mgr.Start(anton); err != nil {
		t.Fatalf("Start: %v", err)
	}

	lb, err := mgr.Lobby(anton)
	if err != nil {
		t.Fatalf("Lobby: %v", err)
	}
	if !lb.HasSession || !lb.IsOpen {
		t.Fatalf("want an open session, got %+v", lb)
	}
	if lb.Count != 1 || !lb.Joined || !lb.IsCreator {
		t.Errorf("creator should auto-join as host: count=%d joined=%v creator=%v", lb.Count, lb.Joined, lb.IsCreator)
	}
	if lb.Required != 4 {
		t.Errorf("required players = %d, want 4 (from seeded activity)", lb.Required)
	}
}

func TestStartRejectsWhenAlreadyOpen(t *testing.T) {
	mgr, users := newTestEnv(t)
	anton := mkUser(t, users, "Anton")
	berta := mkUser(t, users, "Berta")

	if _, err := mgr.Start(anton); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if _, err := mgr.Start(berta); !errors.Is(err, ErrSessionActive) {
		t.Fatalf("second Start error = %v, want ErrSessionActive", err)
	}
}

func TestJoinAndDoubleJoin(t *testing.T) {
	mgr, users := newTestEnv(t)
	anton := mkUser(t, users, "Anton")
	berta := mkUser(t, users, "Berta")

	s, err := mgr.Start(anton)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := mgr.Join(s.ID, berta); err != nil {
		t.Fatalf("Join: %v", err)
	}

	lb, _ := mgr.Lobby(berta)
	if lb.Count != 2 || !lb.Joined {
		t.Errorf("after join: count=%d joined=%v, want 2/true", lb.Count, lb.Joined)
	}
	if err := mgr.Join(s.ID, berta); !errors.Is(err, ErrAlreadyJoined) {
		t.Errorf("double join error = %v, want ErrAlreadyJoined", err)
	}
}

func TestLeaveDecrementsCounter(t *testing.T) {
	mgr, users := newTestEnv(t)
	anton := mkUser(t, users, "Anton")
	berta := mkUser(t, users, "Berta")

	s, _ := mgr.Start(anton)
	if err := mgr.Join(s.ID, berta); err != nil {
		t.Fatalf("Join: %v", err)
	}
	if err := mgr.Leave(s.ID, berta); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	lb, _ := mgr.Lobby(berta)
	if lb.Count != 1 || lb.Joined {
		t.Errorf("after leave: count=%d joined=%v, want 1/false", lb.Count, lb.Joined)
	}
}

func TestLeaveLastParticipantCancels(t *testing.T) {
	mgr, users := newTestEnv(t)
	anton := mkUser(t, users, "Anton")

	s, _ := mgr.Start(anton)
	if err := mgr.Leave(s.ID, anton); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	lb, _ := mgr.Lobby(anton)
	if lb.HasSession {
		t.Errorf("empty session should be cancelled, got %+v", lb)
	}
}

func TestDrawTriggersWhenFull(t *testing.T) {
	mgr, users := newTestEnv(t)
	ids := []string{
		mkUser(t, users, "Anton"),
		mkUser(t, users, "Berta"),
		mkUser(t, users, "Cara"),
		mkUser(t, users, "Dora"),
	}

	s, _ := mgr.Start(ids[0])
	// Third join still leaves the session open at 3/4.
	if err := mgr.Join(s.ID, ids[1]); err != nil {
		t.Fatalf("Join Berta: %v", err)
	}
	if err := mgr.Join(s.ID, ids[2]); err != nil {
		t.Fatalf("Join Cara: %v", err)
	}
	if lb, _ := mgr.Lobby(ids[0]); !lb.IsOpen || lb.IsDrawn {
		t.Fatalf("at 3/4 session should still be open, got %+v", lb)
	}

	// Fourth join fills it and must trigger the draw.
	if err := mgr.Join(s.ID, ids[3]); err != nil {
		t.Fatalf("Join Dora: %v", err)
	}

	lb, _ := mgr.Lobby(ids[0])
	if !lb.IsDrawn || lb.IsOpen {
		t.Fatalf("at 4/4 session should be drawn, got %+v", lb)
	}
	if len(lb.Teams) != 2 {
		t.Fatalf("got %d teams, want 2", len(lb.Teams))
	}

	assigned := map[string]string{}
	for _, team := range lb.Teams {
		if len(team.Members) != 2 {
			t.Errorf("team %s has %d members, want 2", team.Label, len(team.Members))
		}
		for _, m := range team.Members {
			if m.Team != team.Label {
				t.Errorf("member %s carries team %q, expected %q", m.DisplayName, m.Team, team.Label)
			}
			assigned[m.UserID] = team.Label
		}
	}
	if len(assigned) != 4 {
		t.Errorf("got %d players assigned to teams, want 4", len(assigned))
	}

	// A full/drawn session no longer accepts joins.
	eve := mkUser(t, users, "Eve")
	if err := mgr.Join(s.ID, eve); !errors.Is(err, ErrSessionNotOpen) {
		t.Errorf("join after draw error = %v, want ErrSessionNotOpen", err)
	}
}

// fillToDrawn starts a session and joins until the seeded 2v2 activity draws.
// It returns the session and the four participant ids in join order.
func fillToDrawn(t *testing.T, mgr *Manager, users *user.Repository) (*Session, []string) {
	t.Helper()
	ids := []string{
		mkUser(t, users, "Anton"),
		mkUser(t, users, "Berta"),
		mkUser(t, users, "Cara"),
		mkUser(t, users, "Dora"),
	}
	s, err := mgr.Start(ids[0])
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for _, id := range ids[1:] {
		if err := mgr.Join(s.ID, id); err != nil {
			t.Fatalf("Join: %v", err)
		}
	}
	if lb, _ := mgr.Lobby(ids[0]); !lb.IsDrawn {
		t.Fatalf("session should be drawn after 4 joins, got %+v", lb)
	}
	return s, ids
}

func TestStartRejectedWhileDrawn(t *testing.T) {
	mgr, users := newTestEnv(t)
	_, ids := fillToDrawn(t, mgr, users)

	// One table: a drawn (in-progress) match blocks starting a new round.
	if _, err := mgr.Start(ids[0]); !errors.Is(err, ErrSessionActive) {
		t.Fatalf("Start while drawn = %v, want ErrSessionActive", err)
	}
}

func TestFinishRecordsWinnerAndFreesTable(t *testing.T) {
	mgr, users := newTestEnv(t)
	s, ids := fillToDrawn(t, mgr, users)

	// Any participant may end the match (here the non-host Berta).
	if err := mgr.Finish(s.ID, ids[1], "A"); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	done, err := mgr.repo.Get(s.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if done.Status != StatusDone || done.WinnerTeam != "A" {
		t.Errorf("finished session = %q winner=%q, want DONE/A", done.Status, done.WinnerTeam)
	}

	// The table is free again: lobby is empty and a new round can start.
	if lb, _ := mgr.Lobby(ids[0]); lb.HasSession {
		t.Errorf("table should be free after finish, got %+v", lb)
	}
	if _, err := mgr.Start(ids[0]); err != nil {
		t.Errorf("Start after finish = %v, want nil", err)
	}
}

func TestFinishRejectsNonParticipant(t *testing.T) {
	mgr, users := newTestEnv(t)
	s, _ := fillToDrawn(t, mgr, users)
	eve := mkUser(t, users, "Eve")

	if err := mgr.Finish(s.ID, eve, "A"); !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("Finish by non-participant = %v, want ErrNotParticipant", err)
	}
}

func TestFinishRejectsInvalidWinner(t *testing.T) {
	mgr, users := newTestEnv(t)
	s, ids := fillToDrawn(t, mgr, users)

	if err := mgr.Finish(s.ID, ids[0], "Z"); !errors.Is(err, ErrInvalidWinner) {
		t.Fatalf("Finish with bogus team = %v, want ErrInvalidWinner", err)
	}
}

func TestFinishRejectsWhenNotDrawn(t *testing.T) {
	mgr, users := newTestEnv(t)
	anton := mkUser(t, users, "Anton")
	s, _ := mgr.Start(anton) // still OPEN at 1/4

	if err := mgr.Finish(s.ID, anton, "A"); !errors.Is(err, ErrSessionNotDrawn) {
		t.Fatalf("Finish on open session = %v, want ErrSessionNotDrawn", err)
	}
}

// backdateExpiry pushes a session's expires_at into the past so ExpireStale
// treats it as timed out.
func backdateExpiry(t *testing.T, mgr *Manager, sessionID string) {
	t.Helper()
	if _, err := mgr.repo.db.Exec(
		`UPDATE sessions SET expires_at = ? WHERE id = ?`,
		time.Now().Add(-time.Minute).UTC(), sessionID,
	); err != nil {
		t.Fatalf("backdate expiry: %v", err)
	}
}

func TestExpireStaleClosesTimedOutOpenSession(t *testing.T) {
	mgr, users := newTestEnv(t)
	anton := mkUser(t, users, "Anton")

	s, _ := mgr.Start(anton)
	backdateExpiry(t, mgr, s.ID)

	n, err := mgr.ExpireStale()
	if err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if n != 1 {
		t.Fatalf("expired %d sessions, want 1", n)
	}

	got, _ := mgr.repo.Get(s.ID)
	if got.Status != StatusExpired {
		t.Errorf("status = %q, want EXPIRED", got.Status)
	}
	// The table is free again and a new round can start.
	if lb, _ := mgr.Lobby(anton); lb.HasSession {
		t.Errorf("expired session should not be active, got %+v", lb)
	}
}

func TestExpireStaleLeavesDrawnSessions(t *testing.T) {
	mgr, users := newTestEnv(t)
	s, _ := fillToDrawn(t, mgr, users)

	// A drawn (in-progress) match must not be reaped, even past its expiry.
	backdateExpiry(t, mgr, s.ID)

	n, err := mgr.ExpireStale()
	if err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if n != 0 {
		t.Fatalf("expired %d sessions, want 0 (drawn is immune)", n)
	}
	if got, _ := mgr.repo.Get(s.ID); got.Status != StatusDrawn {
		t.Errorf("status = %q, want DRAWN", got.Status)
	}
}

func TestReRollKeepsValidDrawAndStaysDrawn(t *testing.T) {
	mgr, users := newTestEnv(t)
	s, ids := fillToDrawn(t, mgr, users)

	if err := mgr.ReRoll(s.ID, ids[0]); err != nil {
		t.Fatalf("ReRoll by host: %v", err)
	}

	lb, _ := mgr.Lobby(ids[0])
	if !lb.IsDrawn || len(lb.Teams) != 2 {
		t.Fatalf("after re-roll want a drawn 2-team session, got %+v", lb)
	}
	assigned := map[string]string{}
	for _, team := range lb.Teams {
		if len(team.Members) != 2 {
			t.Errorf("team %s has %d members, want 2", team.Label, len(team.Members))
		}
		for _, m := range team.Members {
			assigned[m.UserID] = team.Label
		}
	}
	if len(assigned) != 4 {
		t.Errorf("got %d players assigned, want 4", len(assigned))
	}
}

func TestReRollRejectsNonCreator(t *testing.T) {
	mgr, users := newTestEnv(t)
	s, ids := fillToDrawn(t, mgr, users)

	if err := mgr.ReRoll(s.ID, ids[1]); !errors.Is(err, ErrNotCreator) {
		t.Fatalf("ReRoll by non-host = %v, want ErrNotCreator", err)
	}
}

func TestReRollRejectsWhenNotDrawn(t *testing.T) {
	mgr, users := newTestEnv(t)
	anton := mkUser(t, users, "Anton")
	s, _ := mgr.Start(anton) // OPEN

	if err := mgr.ReRoll(s.ID, anton); !errors.Is(err, ErrSessionNotDrawn) {
		t.Fatalf("ReRoll on open session = %v, want ErrSessionNotDrawn", err)
	}
}

func TestHistoryListsFinishedMatchesAndStats(t *testing.T) {
	mgr, users := newTestEnv(t)
	s, ids := fillToDrawn(t, mgr, users)
	if err := mgr.Finish(s.ID, ids[0], "A"); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	h, err := mgr.History(10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if h.Total != 1 || len(h.Entries) != 1 {
		t.Fatalf("history total=%d entries=%d, want 1/1", h.Total, len(h.Entries))
	}
	e := h.Entries[0]
	if e.WinnerTeam != "A" || len(e.Teams) != 2 {
		t.Errorf("entry winner=%q teams=%d, want A/2", e.WinnerTeam, len(e.Teams))
	}

	if len(h.Stats) != 4 {
		t.Fatalf("leaderboard has %d players, want 4", len(h.Stats))
	}
	var totalWins, totalPlayed int
	for _, st := range h.Stats {
		totalWins += st.Wins
		totalPlayed += st.Played
	}
	if totalPlayed != 4 {
		t.Errorf("total played = %d, want 4", totalPlayed)
	}
	if totalWins != 2 { // the two members of winning team A
		t.Errorf("total wins = %d, want 2", totalWins)
	}
}

func TestHistoryEmptyWhenNothingFinished(t *testing.T) {
	mgr, _ := newTestEnv(t)

	h, err := mgr.History(10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if h.Total != 0 || len(h.Entries) != 0 || len(h.Stats) != 0 {
		t.Errorf("empty history want zeros, got %+v", h)
	}
}
