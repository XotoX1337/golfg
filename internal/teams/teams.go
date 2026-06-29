// Package teams posts session notifications to a Microsoft Teams channel through
// a Power-Automate workflow webhook ("When a Teams webhook request is
// received"). It implements session.Notifier, so the session Manager announces
// events without knowing anything about Teams.
//
// Teams is a pure outbound notification medium here: we POST an Adaptive Card
// and never receive callbacks (all interaction happens in the app). Posts are
// best-effort and run in a background goroutine so a slow or failing webhook
// never blocks the request flow or crashes the app. When no webhook is
// configured the client logs events instead of posting them — the same
// graceful-degradation contract the app keeps for unconfigured SSO.
//
// The payload shape is the one the Power-Automate "Teams webhook" trigger
// expects: a message whose single attachment is an Adaptive Card. See
// docs/setup-teams.md for the matching workflow setup.
package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/XotoX1337/golfg/internal/i18n"
	"github.com/XotoX1337/golfg/internal/session"
	"go.uber.org/zap"
)

// postTimeout caps a single webhook POST so a hung endpoint can't pin a goroutine.
const postTimeout = 10 * time.Second

// Client posts Adaptive Cards to a Power-Automate workflow webhook. The zero
// value is not usable; build one with New.
type Client struct {
	webhookURL string
	baseURL    string // app base URL for deep-links, without trailing slash
	loc        *i18n.Localizer
	http       *http.Client
	logger     *zap.Logger
}

// New builds a Teams client. An empty webhookURL puts the client in log-only
// mode (graceful degradation). baseURL is used to build deep-links back into the
// app. loc fixes the language of the channel notifications (the channel has no
// per-request locale).
func New(webhookURL, baseURL string, loc *i18n.Localizer, logger *zap.Logger) *Client {
	return &Client{
		webhookURL: strings.TrimSpace(webhookURL),
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		loc:        loc,
		http:       &http.Client{Timeout: postTimeout},
		logger:     logger,
	}
}

// t translates a notification message ID in the client's configured language.
func (c *Client) t(id string, pairs ...any) string {
	return c.loc.T(id, pairs...)
}

// SessionStarted posts the "someone wants to play" announcement with a deep-link
// to join.
func (c *Client) SessionStarted(e session.SessionStartedEvent) {
	name := e.Creator.DisplayName
	if name == "" {
		name = "Someone"
	}
	title := c.t("teams_notify_started_title", "Name", name, "Activity", e.Activity.Name)
	sub := c.t("teams_notify_slots_left", "Count", e.FreeSlots)
	if e.FreeSlots == 1 {
		sub = c.t("teams_notify_last_slot")
	}
	msg := c.card([]cardElement{
		textBlock(title, "Large", "Bolder"),
		textBlock(sub, "", ""),
	}, c.t("teams_notify_join_action"))
	c.post("session started", title+" — "+sub, msg)
}

// TeamsDrawn posts the final line-up once the session is full.
func (c *Client) TeamsDrawn(e session.TeamsDrawnEvent) {
	body := []cardElement{textBlock(c.t("teams_notify_drawn_title"), "Large", "Bolder")}
	var summary []string
	for _, t := range e.Teams {
		line := c.teamLine(t)
		body = append(body, textBlock(line, "", "Bolder"))
		summary = append(summary, line)
	}
	msg := c.card(body, c.t("teams_notify_open_action"))
	c.post("teams drawn", strings.Join(summary, " — "), msg)
}

// MatchFinished posts the result once a match is ended in the app: the winning
// team (or a draw) plus the final line-ups for context.
func (c *Client) MatchFinished(r session.MatchResult) {
	var title string
	if r.WinnerTeam == "" {
		title = c.t("teams_notify_finished_tie", "Activity", r.Activity.Name)
	} else {
		title = c.t("teams_notify_winner", "Team", session.TeamName(r.Teams, r.WinnerTeam), "Activity", r.Activity.Name)
	}
	body := []cardElement{textBlock(c.t("teams_notify_finished_title"), "Large", "Bolder"), textBlock(title, "", "Bolder")}
	for _, t := range r.Teams {
		body = append(body, textBlock(c.teamLine(t), "", ""))
	}
	msg := c.card(body, c.t("teams_notify_open_action"))
	c.post("match finished", title, msg)
}

// teamLine renders an "Anton & Berta: Anton Müller, Berta Schmidt" summary line:
// the team's display name followed by its members' full names.
func (c *Client) teamLine(t session.Team) string {
	names := make([]string, 0, len(t.Members))
	for _, m := range t.Members {
		names = append(names, displayName(m))
	}
	return c.t("teams_notify_team_line", "Name", t.Name(), "Names", strings.Join(names, ", "))
}

// post sends msg to the webhook in the background. With no webhook configured it
// logs the human-readable summary and returns. Errors are logged, never
// propagated — a notification failure must not affect the session flow.
func (c *Client) post(event, summary string, msg adaptiveMessage) {
	if c.webhookURL == "" {
		c.logger.Info("teams post (log only, no webhook)",
			zap.String("event", event),
			zap.String("message", summary),
		)
		return
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("teams: marshal card", zap.String("event", event), zap.Error(err))
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), postTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(payload))
		if err != nil {
			c.logger.Error("teams: build request", zap.String("event", event), zap.Error(err))
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			c.logger.Error("teams: post failed", zap.String("event", event), zap.Error(err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			c.logger.Error("teams: unexpected status",
				zap.String("event", event),
				zap.Int("status", resp.StatusCode),
			)
			return
		}
		c.logger.Info("teams post sent", zap.String("event", event), zap.Int("status", resp.StatusCode))
	}()
}

// card wraps the given body elements in the message envelope the Power-Automate
// trigger expects, optionally adding a single deep-link button to the app.
func (c *Client) card(body []cardElement, actionTitle string) adaptiveMessage {
	ac := adaptiveCard{
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Type:    "AdaptiveCard",
		Version: "1.4",
		Body:    body,
	}
	if c.baseURL != "" && actionTitle != "" {
		ac.Actions = []cardAction{{
			Type:  "Action.OpenUrl",
			Title: actionTitle,
			URL:   c.baseURL + "/",
		}}
	}
	return adaptiveMessage{
		Type: "message",
		Attachments: []attachment{{
			ContentType: "application/vnd.microsoft.card.adaptive",
			Content:     ac,
		}},
	}
}

// displayName falls back to the email (or a placeholder) when a user has no
// display name cached yet.
func displayName(p session.Participant) string {
	switch {
	case p.DisplayName != "":
		return p.DisplayName
	case p.Email != "":
		return p.Email
	default:
		return "a player"
	}
}

// textBlock builds an Adaptive Card TextBlock; pass "" for size/weight to omit.
func textBlock(text, size, weight string) cardElement {
	return cardElement{Type: "TextBlock", Text: text, Size: size, Weight: weight, Wrap: true}
}

// --- Adaptive Card payload types (Power-Automate "Teams webhook" format) ---

// adaptiveMessage is the envelope: a message with one Adaptive Card attachment.
type adaptiveMessage struct {
	Type        string       `json:"type"`
	Attachments []attachment `json:"attachments"`
}

type attachment struct {
	ContentType string       `json:"contentType"`
	Content     adaptiveCard `json:"content"`
}

type adaptiveCard struct {
	Schema  string        `json:"$schema"`
	Type    string        `json:"type"`
	Version string        `json:"version"`
	Body    []cardElement `json:"body"`
	Actions []cardAction  `json:"actions,omitempty"`
}

type cardElement struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Size   string `json:"size,omitempty"`
	Weight string `json:"weight,omitempty"`
	Wrap   bool   `json:"wrap,omitempty"`
}

type cardAction struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}
