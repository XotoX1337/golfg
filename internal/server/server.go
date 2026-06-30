// Package server wires the Fiber app: template engine, static assets and routes.
package server

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/XotoX1337/golfg/internal/auth"
	"github.com/XotoX1337/golfg/internal/config"
	"github.com/XotoX1337/golfg/internal/i18n"
	"github.com/XotoX1337/golfg/internal/session"
	"github.com/XotoX1337/golfg/internal/store"
	"github.com/XotoX1337/golfg/internal/teams"
	"github.com/XotoX1337/golfg/internal/user"
	"github.com/XotoX1337/golfg/web"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/template/html/v2"
	"go.uber.org/zap"
)

// Server bundles the Fiber app with its dependencies.
type Server struct {
	app      *fiber.App
	cfg      *config.Config
	store    *store.Store
	logger   *zap.Logger
	auth     *auth.Manager
	sessions *session.Manager
	users    *user.Repository
	i18n     *i18n.Bundle

	reaperStop chan struct{} // closed on Shutdown to stop the expiry reaper
}

// reapInterval is how often the expiry reaper sweeps for stale OPEN sessions.
// A minute is plenty given expiry is configured in whole minutes.
const reapInterval = time.Minute

// New constructs the Fiber app, registers the template engine, routes and the
// embedded static file handler.
func New(cfg *config.Config, st *store.Store, logger *zap.Logger) (*Server, error) {
	// Strip the "templates/" prefix so render paths are e.g. "index/show".
	tmplFS, err := fs.Sub(web.Templates, "templates")
	if err != nil {
		return nil, err
	}
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		return nil, err
	}

	bundle, err := i18n.New()
	if err != nil {
		return nil, err
	}

	engine := html.NewFileSystem(http.FS(tmplFS), ".html")
	engine.AddFunc("Name", cfg.AppName)
	engine.AddFunc("Accent", cfg.AccentColor)
	engine.AddFunc("PlayCTA", cfg.PlayCTA)
	engine.AddFunc("Initials", initials)
	engine.AddFunc("OpenSlots", openSlots)
	engine.AddFunc("Version", func() string { return config.Version })
	engine.AddFunc("Year", func() int { return time.Now().Year() })
	engine.AddFunc("DateTime", func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Local().Format("2006-01-02 15:04")
	})
	engine.AddFunc("dict", dict)

	app := fiber.New(fiber.Config{
		AppName: cfg.AppName(),
		Views:   engine,
	})

	authMgr, err := auth.New(cfg, st, logger)
	if err != nil {
		return nil, err
	}

	// Teams notifier: posts session events to the configured webhook, or logs
	// them when none is set (graceful degradation). The channel has no
	// per-request locale, so its language is fixed by config.
	notifier := teams.New(cfg.Teams.WebhookURL, cfg.App.BaseURL, cfg.PlayAnnouncement(), cfg.Teams.MentionPlayers, bundle.Localizer(cfg.Teams.Lang), logger)
	sessionMgr := session.New(st, logger, cfg.Session.ExpireMinutes, session.WithNotifier(notifier))

	s := &Server{app: app, cfg: cfg, store: st, logger: logger, auth: authMgr, sessions: sessionMgr, users: user.NewRepository(st), i18n: bundle}
	s.routes(staticFS)
	if cfg.Session.ExpireMinutes > 0 {
		s.startReaper()
	}
	return s, nil
}

// startReaper launches a background goroutine that periodically expires stale
// OPEN sessions. It sweeps once immediately (to clear rounds left over from a
// previous run) and then on every tick until Shutdown closes reaperStop.
func (s *Server) startReaper() {
	s.reaperStop = make(chan struct{})
	go func() {
		sweep := func() {
			if n, err := s.sessions.ExpireStale(); err != nil {
				s.logger.Error("expire stale sessions", zap.Error(err))
			} else if n > 0 {
				s.logger.Info("expired stale sessions", zap.Int("count", n))
			}
		}
		sweep()

		t := time.NewTicker(reapInterval)
		defer t.Stop()
		for {
			select {
			case <-s.reaperStop:
				return
			case <-t.C:
				sweep()
			}
		}
	}()
}

// initials returns up to two uppercase letters for an avatar badge: the first
// letter of the first and last name parts (e.g. "Anton Berg" -> "AB", "Anton"
// -> "A"). Falls back to "?" for an empty/symbol-only name.
func initials(name string) string {
	fields := strings.Fields(name)
	var letters []rune
	for _, f := range fields {
		for _, r := range f {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				letters = append(letters, unicode.ToUpper(r))
				break
			}
		}
	}
	switch len(letters) {
	case 0:
		return "?"
	case 1:
		return string(letters[0])
	default:
		return string([]rune{letters[0], letters[len(letters)-1]})
	}
}

// dict builds a map from alternating key/value template arguments, so a partial
// can be handed several named values at once (e.g. a translator plus a data
// slice) — Go templates pass a single pipeline value to {{ template }}.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, errors.New("dict: needs an even number of arguments")
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, errors.New("dict: keys must be strings")
		}
		m[key] = pairs[i+1]
	}
	return m, nil
}

// openSlots returns a slice sized to the number of still-open player slots, so
// templates can render placeholder rows (range over it) and keep the lobby
// layout stable as players join.
func openSlots(required, count int) []struct{} {
	n := required - count
	if n < 0 {
		n = 0
	}
	return make([]struct{}, n)
}

func (s *Server) routes(staticFS fs.FS) {
	// Resolve the request language before anything renders, so the login page
	// (registered below) and every protected view get translations.
	s.app.Use(s.withLocale)

	// Resolve the current user for every request so handlers and templates
	// (header, logout link) can see who is logged in.
	s.app.Use(s.auth.LoadUser)

	s.auth.RegisterRoutes(s.app)

	// Protected app routes require a logged-in user.
	s.app.Get("/", s.auth.RequireAuth, s.showIndex)
	s.app.Get("/session", s.auth.RequireAuth, s.showSessionFragment)
	s.app.Post("/session/start", s.auth.RequireAuth, s.startSession)
	s.app.Post("/session/join", s.auth.RequireAuth, s.joinSession)
	s.app.Post("/session/leave", s.auth.RequireAuth, s.leaveSession)
	s.app.Post("/session/reroll", s.auth.RequireAuth, s.reRollSession)
	s.app.Get("/session/finish", s.auth.RequireAuth, s.showFinishModal)
	s.app.Post("/session/finish", s.auth.RequireAuth, s.finishSession)
	s.app.Get("/history", s.auth.RequireAuth, s.showHistory)
	s.app.Get("/avatar/:userID", s.auth.RequireAuth, s.showAvatar)

	// Serve embedded static assets as a catch-all (must be registered last).
	s.app.Use("/", filesystem.New(filesystem.Config{
		Root: http.FS(staticFS),
	}))
}

// Listen binds to the configured host:port and blocks.
func (s *Server) Listen() error {
	s.logger.Info("listening", zap.String("addr", s.cfg.Addr()))
	return s.app.Listen(s.cfg.Addr())
}

// Shutdown gracefully stops the server, first signalling the expiry reaper to
// stop.
func (s *Server) Shutdown() error {
	if s.reaperStop != nil {
		close(s.reaperStop)
	}
	return s.app.Shutdown()
}
