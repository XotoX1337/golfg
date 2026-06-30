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
	"text/template"
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
	webhookURL     string
	baseURL        string // app base URL for deep-links, without trailing slash
	loc            *i18n.Localizer
	startedTmpl    *template.Template // optional custom "session started" headline
	mentionPlayers bool               // @-mention drawn players in the "teams are set" post
	http           *http.Client
	logger         *zap.Logger
}

// New builds a Teams client. An empty webhookURL puts the client in log-only
// mode (graceful degradation). baseURL is used to build deep-links back into the
// app. loc fixes the language of the channel notifications (the channel has no
// per-request locale). playAnnouncement, when non-empty, is a text/template with
// a single {{.Name}} placeholder that overrides the localized "session started"
// headline; an invalid template is logged and ignored (the default is used).
// mentionPlayers enables @-mentions of the drawn players in the "teams are set"
// post (members with an Entra object id only — see TeamsDrawn).
func New(webhookURL, baseURL, playAnnouncement string, mentionPlayers bool, loc *i18n.Localizer, logger *zap.Logger) *Client {
	var startedTmpl *template.Template
	if s := strings.TrimSpace(playAnnouncement); s != "" {
		t, err := template.New("play_announcement").Parse(s)
		if err != nil {
			logger.Warn("teams: invalid branding.play_announcement template, using localized default", zap.Error(err))
		} else {
			startedTmpl = t
		}
	}
	return &Client{
		webhookURL:     strings.TrimSpace(webhookURL),
		baseURL:        strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		loc:            loc,
		startedTmpl:    startedTmpl,
		mentionPlayers: mentionPlayers,
		http:           &http.Client{Timeout: postTimeout},
		logger:         logger,
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
	title := c.startedTitle(name, e.Activity.Name)
	sub := c.t("teams_notify_slots_left", "Count", e.FreeSlots)
	if e.FreeSlots == 1 {
		sub = c.t("teams_notify_last_slot")
	}
	msg := c.card([]cardElement{
		textBlock(title, "Large", "Bolder"),
		textBlock(sub, "", ""),
	}, c.t("teams_notify_join_action"), nil)
	c.post("session started", title+" — "+sub, msg)
}

// startedTitle renders the "session started" headline. With a configured
// branding.play_announcement it renders that fixed template against the creator's
// {{.Name}} (text/template, so the value is substituted as plain text). Without
// one — or if rendering fails — it falls back to the localized default, which
// also carries the activity name.
func (c *Client) startedTitle(name, activity string) string {
	fallback := func() string {
		return c.t("teams_notify_started_title", "Name", name, "Activity", activity)
	}
	if c.startedTmpl == nil {
		return fallback()
	}
	var buf bytes.Buffer
	if err := c.startedTmpl.Execute(&buf, map[string]string{"Name": name}); err != nil {
		c.logger.Warn("teams: render branding.play_announcement failed, using localized default", zap.Error(err))
		return fallback()
	}
	return buf.String()
}

// TeamsDrawn posts the final line-up once the session is full. When
// mentionPlayers is on, each drawn member with an Entra object id is rendered as
// an @-mention so they get a real Teams notification; members without one (dev
// login) fall back to a plain name. The card carries the matching msteams
// mention entities (see mentionTeamLine). The log summary always uses plain
// names so the OID tokens never leak into the log.
func (c *Client) TeamsDrawn(e session.TeamsDrawnEvent) {
	body := []cardElement{textBlock(c.t("teams_notify_drawn_title"), "Large", "Bolder")}
	var summary []string
	var mentions []mentionEntity
	for _, t := range e.Teams {
		summary = append(summary, c.teamLine(t))
		if c.mentionPlayers {
			line, ents := c.mentionTeamLine(t)
			body = append(body, textBlock(line, "", "Bolder"))
			mentions = append(mentions, ents...)
		} else {
			body = append(body, textBlock(c.teamLine(t), "", "Bolder"))
		}
	}
	msg := c.card(body, c.t("teams_notify_open_action"), mentions)
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
	msg := c.card(body, c.t("teams_notify_open_action"), nil)
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

// mentionTeamLine renders the same "Name: members" line as teamLine, but replaces
// each member that has an Entra object id with an "<at>Display Name</at>" mention
// token and returns the matching mention entities to attach to the card. Members
// without an OID (dev login, legacy data) stay plain names — never a broken or
// empty token. Token text and entity text are kept byte-identical (Teams only
// renders the mention when they match exactly).
func (c *Client) mentionTeamLine(t session.Team) (string, []mentionEntity) {
	names := make([]string, 0, len(t.Members))
	var ents []mentionEntity
	for _, m := range t.Members {
		name := displayName(m)
		if m.EntraOID == "" {
			names = append(names, name)
			continue
		}
		token := "<at>" + name + "</at>"
		names = append(names, token)
		ents = append(ents, mentionEntity{
			Type:      "mention",
			Text:      token,
			Mentioned: mentioned{ID: m.EntraOID, Name: name},
		})
	}
	line := c.t("teams_notify_team_line", "Name", t.Name(), "Names", strings.Join(names, ", "))
	return line, ents
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
// trigger expects, optionally adding a single deep-link button to the app and a
// set of @-mention entities (attached to the card's msteams property, which is
// where Teams resolves the <at>…</at> tokens in the body).
func (c *Client) card(body []cardElement, actionTitle string, mentions []mentionEntity) adaptiveMessage {
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
	if len(mentions) > 0 {
		ac.Msteams = &msteamsProps{Entities: mentions}
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
	// Msteams carries Teams-specific extensions (here: @-mention entities). It
	// belongs on the card content, not the message envelope. Omitted when there
	// are no mentions.
	Msteams *msteamsProps `json:"msteams,omitempty"`
}

// msteamsProps is the card's "msteams" extension object. entities[] declares the
// @-mentions whose <at>…</at> tokens appear in the body text.
type msteamsProps struct {
	Entities []mentionEntity `json:"entities"`
}

// mentionEntity binds an <at>Name</at> token in the body to a real user. Text
// must match the body token character-for-character or Teams won't render it.
type mentionEntity struct {
	Type      string    `json:"type"` // always "mention"
	Text      string    `json:"text"` // the <at>…</at> token, byte-identical to the body
	Mentioned mentioned `json:"mentioned"`
}

// mentioned identifies the user to ping. ID is the Entra object id (the Teams
// webhook also accepts a UPN/email); Name is the display name.
type mentioned struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
