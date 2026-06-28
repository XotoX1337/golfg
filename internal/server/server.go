// Package server wires the Fiber app: template engine, static assets and routes.
package server

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/XotoX1337/golfg/internal/auth"
	"github.com/XotoX1337/golfg/internal/config"
	"github.com/XotoX1337/golfg/internal/i18n"
	"github.com/XotoX1337/golfg/internal/session"
	"github.com/XotoX1337/golfg/internal/store"
	"github.com/XotoX1337/golfg/internal/teams"
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
	engine.AddFunc("Name", func() string { return config.DisplayName })
	engine.AddFunc("Version", func() string { return config.Version })
	engine.AddFunc("Year", func() int { return time.Now().Year() })
	engine.AddFunc("DateTime", func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Local().Format("2006-01-02 15:04")
	})

	app := fiber.New(fiber.Config{
		AppName: config.DisplayName,
		Views:   engine,
	})

	authMgr, err := auth.New(cfg, st, logger)
	if err != nil {
		return nil, err
	}

	// Teams notifier: posts session events to the configured webhook, or logs
	// them when none is set (graceful degradation).
	notifier := teams.New(cfg.Teams.WebhookURL, cfg.App.BaseURL, logger)
	sessionMgr := session.New(st, logger, cfg.Session.ExpireMinutes, session.WithNotifier(notifier))

	s := &Server{app: app, cfg: cfg, store: st, logger: logger, auth: authMgr, sessions: sessionMgr, i18n: bundle}
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
