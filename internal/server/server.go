// Package server wires the Fiber app: template engine, static assets and routes.
package server

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/XotoX1337/golfg/internal/auth"
	"github.com/XotoX1337/golfg/internal/config"
	"github.com/XotoX1337/golfg/internal/session"
	"github.com/XotoX1337/golfg/internal/store"
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
}

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

	engine := html.NewFileSystem(http.FS(tmplFS), ".html")
	engine.AddFunc("Name", func() string { return config.DisplayName })
	engine.AddFunc("Version", func() string { return config.Version })
	engine.AddFunc("Year", func() int { return time.Now().Year() })

	app := fiber.New(fiber.Config{
		AppName: config.DisplayName,
		Views:   engine,
	})

	authMgr, err := auth.New(cfg, st, logger)
	if err != nil {
		return nil, err
	}

	sessionMgr := session.New(st, logger, cfg.Session.ExpireMinutes)

	s := &Server{app: app, cfg: cfg, store: st, logger: logger, auth: authMgr, sessions: sessionMgr}
	s.routes(staticFS)
	return s, nil
}

func (s *Server) routes(staticFS fs.FS) {
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
	s.app.Get("/session/finish", s.auth.RequireAuth, s.showFinishModal)
	s.app.Post("/session/finish", s.auth.RequireAuth, s.finishSession)

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

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}
