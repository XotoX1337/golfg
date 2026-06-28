package teams

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/XotoX1337/golfg/internal/activity"
	"github.com/XotoX1337/golfg/internal/session"
	"go.uber.org/zap"
)

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
	c := New(srv.URL, "https://kicker.intranet/", zap.NewNop())

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
	c := New(srv.URL, "https://kicker.intranet", zap.NewNop())

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

// With no webhook configured the client must not call out (log-only mode) and
// must never block or panic.
func TestNoWebhookIsLogOnly(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer srv.Close()

	c := New("", "https://kicker.intranet", zap.NewNop())
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
