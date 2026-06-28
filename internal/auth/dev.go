package auth

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// handleLogin starts SSO when configured, otherwise shows the dev login form.
func (m *Manager) handleLogin(c *fiber.Ctx) error {
	if m.CurrentUser(c) != nil {
		return c.Redirect("/", fiber.StatusFound)
	}
	if m.cfg.AuthEnabled() {
		return m.startSSO(c)
	}
	return m.renderDevLogin(c, "")
}

// handleDevLogin processes the local pseudo-login (no SSO). Anyone can pick a
// name; it exists only so the public repo is testable without an Entra tenant.
func (m *Manager) handleDevLogin(c *fiber.Ctx) error {
	name := strings.TrimSpace(c.FormValue("name"))
	email := strings.TrimSpace(c.FormValue("email"))
	if name == "" {
		return m.renderDevLogin(c, "login_err_name_required")
	}

	u, err := m.users.UpsertDev(name, email)
	if err != nil {
		m.logger.Error("dev login", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "could not create user")
	}
	if err := m.login(c, u); err != nil {
		return err
	}
	m.logger.Info("user logged in via dev login", zap.String("user", u.DisplayName))
	return c.Redirect("/", fiber.StatusFound)
}

// handleLogout destroys the session and returns to the home page.
func (m *Manager) handleLogout(c *fiber.Ctx) error {
	if sess, err := m.sessions.Get(c); err == nil {
		if err := sess.Destroy(); err != nil {
			m.logger.Warn("destroy session", zap.Error(err))
		}
	}
	return c.Redirect("/", fiber.StatusFound)
}

// renderDevLogin renders the dev login form with an optional error. errKey is
// an i18n message ID (empty for none); the template translates it for display.
func (m *Manager) renderDevLogin(c *fiber.Ctx, errKey string) error {
	return c.Render("auth/login", fiber.Map{"ErrorKey": errKey})
}
