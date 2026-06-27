package server

import "github.com/gofiber/fiber/v2"

// showIndex renders the lobby placeholder. This is the WP0 smoke-test route;
// the real, state-driven lobby arrives with the core loop (WP2).
func (s *Server) showIndex(c *fiber.Ctx) error {
	return c.Render("index/show", fiber.Map{})
}
