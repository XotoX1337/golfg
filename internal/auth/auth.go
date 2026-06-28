// Package auth handles user authentication: Microsoft Entra ID (OIDC) login with
// a server-side session, plus a local dev login that keeps the public repo
// usable without an Entra tenant. When [auth] is unconfigured the manager runs
// in dev mode; otherwise it performs a full OIDC authorization-code flow.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/XotoX1337/golfg/internal/config"
	"github.com/XotoX1337/golfg/internal/store"
	"github.com/XotoX1337/golfg/internal/user"
)

// Session keys. The user id is set on successful login; the OIDC state/nonce are
// short-lived values used to tie a callback to the request that started it.
const (
	sessionUserKey = "user_id"
	oidcStateKey   = "oidc_state"
	oidcNonceKey   = "oidc_nonce"

	localKeyUser = "currentUser" // c.Locals key for the resolved *user.User
)

// Manager owns the session store and (in SSO mode) the OIDC client.
type Manager struct {
	cfg      *config.Config
	users    *user.Repository
	logger   *zap.Logger
	sessions *session.Store

	// OIDC client, nil in dev mode.
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// New builds the auth manager. In SSO mode it contacts the Entra discovery
// endpoint to configure the OIDC client; in dev mode it skips that entirely.
func New(cfg *config.Config, st *store.Store, logger *zap.Logger) (*Manager, error) {
	m := &Manager{
		cfg:    cfg,
		users:  user.NewRepository(st),
		logger: logger,
		sessions: session.New(session.Config{
			Expiration:     7 * 24 * time.Hour,
			KeyLookup:      "cookie:golfg_session",
			CookieHTTPOnly: true,
			CookieSecure:   cfg.Session.CookieSecure,
			CookieSameSite: "Lax", // allow the cookie on the top-level OIDC callback redirect
		}),
	}

	if cfg.AuthEnabled() {
		if err := m.initOIDC(context.Background()); err != nil {
			return nil, fmt.Errorf("init oidc: %w", err)
		}
		logger.Info("auth: Entra SSO enabled")
	} else {
		logger.Warn("auth: running in DEV mode — no SSO configured, local pseudo-login active")
	}
	return m, nil
}

// RegisterRoutes mounts the auth endpoints. /auth/login starts the flow (real
// SSO redirect or the dev login form), /auth/callback completes SSO, and
// /auth/logout clears the session.
func (m *Manager) RegisterRoutes(router fiber.Router) {
	router.Get("/auth/login", m.handleLogin)
	router.Get("/auth/callback", m.handleCallback)
	router.Get("/auth/logout", m.handleLogout)
	router.Post("/auth/logout", m.handleLogout)
	if !m.cfg.AuthEnabled() {
		router.Post("/auth/dev-login", m.handleDevLogin)
	}
}

// LoadUser resolves the logged-in user (if any) from the session and exposes it
// to handlers (via Locals) and templates (via the view bind map), so the header
// can render the current user and a logout link everywhere.
func (m *Manager) LoadUser(c *fiber.Ctx) error {
	u := m.currentUser(c)
	c.Locals(localKeyUser, u)
	c.Bind(fiber.Map{ //nolint:errcheck // Bind only errors on a nil ctx
		"CurrentUser": u,
		"DevMode":     !m.cfg.AuthEnabled(),
	})
	return c.Next()
}

// RequireAuth blocks unauthenticated access, redirecting browsers (and htmx
// requests, via HX-Redirect) to the login page. It relies on LoadUser running
// earlier in the chain.
func (m *Manager) RequireAuth(c *fiber.Ctx) error {
	if u, ok := c.Locals(localKeyUser).(*user.User); ok && u != nil {
		return c.Next()
	}
	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/auth/login")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	return c.Redirect("/auth/login", fiber.StatusFound)
}

// CurrentUser returns the authenticated user for the request, or nil.
func (m *Manager) CurrentUser(c *fiber.Ctx) *user.User {
	u, _ := c.Locals(localKeyUser).(*user.User)
	return u
}

// currentUser reads the user id from the session and loads the cached record.
func (m *Manager) currentUser(c *fiber.Ctx) *user.User {
	sess, err := m.sessions.Get(c)
	if err != nil {
		return nil
	}
	id, ok := sess.Get(sessionUserKey).(string)
	if !ok || id == "" {
		return nil
	}
	u, err := m.users.GetByID(id)
	if err != nil {
		m.logger.Warn("load session user", zap.Error(err))
		return nil
	}
	return u
}

// login persists the user id in a freshly-regenerated session (guarding against
// session fixation) and writes the cookie.
func (m *Manager) login(c *fiber.Ctx, u *user.User) error {
	sess, err := m.sessions.Get(c)
	if err != nil {
		return err
	}
	if err := sess.Regenerate(); err != nil {
		return err
	}
	sess.Set(sessionUserKey, u.ID)
	return sess.Save()
}

// randomToken returns a URL-safe random string for OIDC state/nonce.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
