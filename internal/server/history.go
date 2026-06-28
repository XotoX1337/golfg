package server

import "github.com/gofiber/fiber/v2"

// historyLimit caps how many finished matches the history page lists; the stats
// counters still cover all matches.
const historyLimit = 20

// showHistory renders the history/stats page: recent finished matches and a
// per-player leaderboard.
func (s *Server) showHistory(c *fiber.Ctx) error {
	h, err := s.sessions.History(historyLimit)
	if err != nil {
		return err
	}
	return c.Render("history/show", fiber.Map{"History": h})
}
