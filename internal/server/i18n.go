package server

import (
	"github.com/gofiber/fiber/v2"
)

// langCookie persists an explicit language choice across requests.
const langCookie = "golfg_lang"

// withLocale resolves the request language and exposes a translator to both
// templates (via the T/Lang bind vars) and handlers (via Locals). Precedence:
// an explicit ?lang override (also persisted as a cookie), then the cookie,
// then the browser's Accept-Language header. Templates call it as
// {{ call .T "message_id" }}; the resolved code is available as {{ .Lang }}.
func (s *Server) withLocale(c *fiber.Ctx) error {
	var prefs []string
	if q := c.Query("lang"); q != "" {
		c.Cookie(&fiber.Cookie{Name: langCookie, Value: q, Path: "/", MaxAge: 365 * 24 * 60 * 60})
		prefs = append(prefs, q)
	} else if ck := c.Cookies(langCookie); ck != "" {
		prefs = append(prefs, ck)
	}
	prefs = append(prefs, c.Get("Accept-Language"))

	loc := s.i18n.Localizer(prefs...)
	c.Locals(localeKey, loc)
	c.Bind(fiber.Map{ //nolint:errcheck // Bind only errors on a nil ctx
		"T":    loc.T,
		"Lang": loc.Lang,
	})
	return c.Next()
}

// localeKey is the c.Locals key for the request's *i18n.Localizer.
const localeKey = "localizer"
