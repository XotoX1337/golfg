package teams

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/XotoX1337/golfg/internal/activity"
	"github.com/XotoX1337/golfg/internal/i18n"
	"github.com/XotoX1337/golfg/internal/session"
	"go.uber.org/zap"
)

// testLocalizer builds an English notification translator for the tests.
func testLocalizer(t *testing.T) *i18n.Localizer {
	t.Helper()
	b, err := i18n.New()
	if err != nil {
		t.Fatalf("load i18n bundle: %v", err)
	}
	return b.Localizer("en")
}

// captureServer returns a test webhook that pushes each received body onto a channel.
func captureServer(t *testing.T) (*httptest.Server, chan []byte) {
	t.Helper()
	bodies := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		bodies <- buf
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, bodies
}

// decode parses a captured body into the message envelope for assertions.
func decode(t *testing.T, body []byte) adaptiveMessage {
	t.Helper()
	var msg adaptiveMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("unmarshal card: %v", err)
	}
	return msg
}

func TestSessionStartedPostsCard(t *testing.T) {
	srv, bodies := captureServer(t)
	c := New(srv.URL, "https://kicker.intranet/", "", testLocalizer(t), zap.NewNop())

	c.SessionStarted(session.SessionStartedEvent{
		Session:   &session.Session{ID: "s1"},
		Activity:  &activity.Activity{Name: "Tischfußball"},
		Creator:   session.Participant{DisplayName: "Anton"},
		Count:     1,
		Required:  4,
		FreeSlots: 3,
	})

	select {
	case body := <-bodies:
		msg := decode(t, body)
		if msg.Type != "message" || len(msg.Attachments) != 1 {
			t.Fatalf("unexpected envelope: %+v", msg)
		}
		card := msg.Attachments[0].Content
		if card.Type != "AdaptiveCard" {
			t.Errorf("contentType card: got %q", card.Type)
		}
		if len(card.Actions) != 1 || card.Actions[0].URL != "https://kicker.intranet/" {
			t.Errorf("expected one deep-link action to the app, got %+v", card.Actions)
		}
		if len(card.Body) == 0 || card.Body[0].Text == "" {
			t.Errorf("expected a non-empty title block, got %+v", card.Body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no webhook request received")
	}
}

func TestTeamsDrawnListsBothTeams(t *testing.T) {
	srv, bodies := captureServer(t)
	c := New(srv.URL, "https://kicker.intranet", "", testLocalizer(t), zap.NewNop())

	c.TeamsDrawn(session.TeamsDrawnEvent{
		Session:  &session.Session{ID: "s1"},
		Activity: &activity.Activity{Name: "Tischfußball"},
		Teams: []session.Team{
			{Label: "A", Members: []session.Participant{{DisplayName: "Anton"}, {DisplayName: "Berta"}}},
			{Label: "B", Members: []session.Participant{{DisplayName: "Carl"}, {DisplayName: "Dora"}}},
		},
	})

	select {
	case body := <-bodies:
		card := decode(t, body).Attachments[0].Content
		// title + one block per team
		if len(card.Body) != 3 {
			t.Fatalf("expected title + 2 team lines, got %d blocks: %+v", len(card.Body), card.Body)
		}
		if card.Body[1].Text == "" || card.Body[2].Text == "" {
			t.Errorf("team lines should be populated: %+v", card.Body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no webhook request received")
	}
}

func TestMatchFinishedPostsWinner(t *testing.T) {
	srv, bodies := captureServer(t)
	c := New(srv.URL, "https://kicker.intranet", "", testLocalizer(t), zap.NewNop())

	c.MatchFinished(session.MatchResult{
		Session:    &session.Session{ID: "s1"},
		Activity:   &activity.Activity{Name: "Tischfußball"},
		WinnerTeam: "A",
		Teams: []session.Team{
			{Label: "A", Members: []session.Participant{{DisplayName: "Anton"}}},
			{Label: "B", Members: []session.Participant{{DisplayName: "Berta"}}},
		},
	})

	select {
	case body := <-bodies:
		card := decode(t, body).Attachments[0].Content
		// header + winner line + one line per team
		if len(card.Body) != 4 {
			t.Fatalf("expected header + winner + 2 team lines, got %d: %+v", len(card.Body), card.Body)
		}
		if !strings.Contains(card.Body[1].Text, "Anton") {
			t.Errorf("winner line should name the winning team: %q", card.Body[1].Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no webhook request received")
	}
}

// With no webhook configured the client must not call out (log-only mode) and
// must never block or panic.
func TestNoWebhookIsLogOnly(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer srv.Close()

	c := New("", "https://kicker.intranet", "", testLocalizer(t), zap.NewNop())
	c.SessionStarted(session.SessionStartedEvent{
		Session:  &session.Session{ID: "s1"},
		Activity: &activity.Activity{Name: "Tischfußball"},
		Creator:  session.Participant{DisplayName: "Anton"},
	})
	time.Sleep(50 * time.Millisecond)
	if called {
		t.Fatal("log-only client must not make HTTP calls")
	}
}

// A configured play_announcement overrides the localized headline, substituting
// the creator's {{.Name}}; the subtitle ("n spots left") stays untouched.
func TestPlayAnnouncementOverridesTitle(t *testing.T) {
	srv, bodies := captureServer(t)
	c := New(srv.URL, "https://kicker.intranet", "{{.Name}} will kickern!", testLocalizer(t), zap.NewNop())

	c.SessionStarted(session.SessionStartedEvent{
		Session:   &session.Session{ID: "s1"},
		Activity:  &activity.Activity{Name: "Tischfußball"},
		Creator:   session.Participant{DisplayName: "Frederic Leist"},
		FreeSlots: 3,
	})

	select {
	case body := <-bodies:
		card := decode(t, body).Attachments[0].Content
		if got := card.Body[0].Text; got != "Frederic Leist will kickern!" {
			t.Errorf("custom title: got %q", got)
		}
		if !strings.Contains(card.Body[1].Text, "3") {
			t.Errorf("subtitle should still report free slots: %q", card.Body[1].Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no webhook request received")
	}
}

// An invalid template must not crash; the client falls back to the localized
// default headline (which carries the activity name).
func TestPlayAnnouncementInvalidFallsBack(t *testing.T) {
	srv, bodies := captureServer(t)
	c := New(srv.URL, "https://kicker.intranet", "{{.Name", testLocalizer(t), zap.NewNop())

	c.SessionStarted(session.SessionStartedEvent{
		Session:   &session.Session{ID: "s1"},
		Activity:  &activity.Activity{Name: "Tischfußball"},
		Creator:   session.Participant{DisplayName: "Anton"},
		FreeSlots: 3,
	})

	select {
	case body := <-bodies:
		card := decode(t, body).Attachments[0].Content
		if !strings.Contains(card.Body[0].Text, "Tischfußball") {
			t.Errorf("expected localized default headline with activity, got %q", card.Body[0].Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no webhook request received")
	}
}
