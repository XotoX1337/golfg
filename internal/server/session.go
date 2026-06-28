package server

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/XotoX1337/golfg/internal/session"
)

// showIndex renders the full lobby page (header/footer + the live session
// fragment). The fragment then keeps itself fresh via htmx polling.
func (s *Server) showIndex(c *fiber.Ctx) error {
	lb, err := s.lobby(c)
	if err != nil {
		return err
	}
	return c.Render("index/show", fiber.Map{"Lobby": lb})
}

// showSessionFragment renders just the live session fragment. This is the
// endpoint htmx polls ("every 3s") and the target of the action buttons.
func (s *Server) showSessionFragment(c *fiber.Ctx) error {
	return s.renderFragment(c)
}

// startSession opens a new session for the current user, then re-renders the
// fragment. A lost race (someone already started) just shows the current state.
func (s *Server) startSession(c *fiber.Ctx) error {
	u := s.auth.CurrentUser(c)
	if _, err := s.sessions.Start(u.ID); err != nil && !isSoftSessionErr(err) {
		s.logger.Error("start session", zap.Error(err))
		return err
	}
	return s.renderFragment(c)
}

// joinSession adds the current user to the open session, then re-renders.
func (s *Server) joinSession(c *fiber.Ctx) error {
	u := s.auth.CurrentUser(c)
	if err := s.sessions.Join(c.FormValue("session_id"), u.ID); err != nil && !isSoftSessionErr(err) {
		s.logger.Error("join session", zap.Error(err))
		return err
	}
	return s.renderFragment(c)
}

// leaveSession removes the current user from the open session, then re-renders.
func (s *Server) leaveSession(c *fiber.Ctx) error {
	u := s.auth.CurrentUser(c)
	if err := s.sessions.Leave(c.FormValue("session_id"), u.ID); err != nil && !isSoftSessionErr(err) {
		s.logger.Error("leave session", zap.Error(err))
		return err
	}
	return s.renderFragment(c)
}

// showFinishModal renders the "who won?" dialog for a DRAWN session. The template
// only emits a dialog when the current user is a participant of a drawn session;
// otherwise it renders empty (clearing the modal container).
func (s *Server) showFinishModal(c *fiber.Ctx) error {
	lb, err := s.lobby(c)
	if err != nil {
		return err
	}
	return c.Render("session/finish", fiber.Map{"Lobby": lb})
}

// finishSession ends the DRAWN session with the picked winning team, then returns
// the refreshed session fragment plus an out-of-band swap that clears the modal.
func (s *Server) finishSession(c *fiber.Ctx) error {
	u := s.auth.CurrentUser(c)
	err := s.sessions.Finish(c.FormValue("session_id"), u.ID, c.FormValue("winner"))
	if err != nil && !isSoftSessionErr(err) {
		s.logger.Error("finish session", zap.Error(err))
		return err
	}
	lb, err := s.lobby(c)
	if err != nil {
		return err
	}
	return c.Render("session/finished", fiber.Map{"Lobby": lb})
}

// renderFragment renders the current lobby state as the standalone fragment.
func (s *Server) renderFragment(c *fiber.Ctx) error {
	lb, err := s.lobby(c)
	if err != nil {
		return err
	}
	return c.Render("session/fragment", fiber.Map{"Lobby": lb})
}

// lobby builds the view model for the current user.
func (s *Server) lobby(c *fiber.Ctx) (*session.Lobby, error) {
	u := s.auth.CurrentUser(c)
	id := ""
	if u != nil {
		id = u.ID
	}
	return s.sessions.Lobby(id)
}

// isSoftSessionErr reports whether the error is an expected race or a benign
// client mismatch (someone acted first, or a stale form), in which case the
// handler simply re-renders the current state instead of surfacing a 500.
func isSoftSessionErr(err error) bool {
	switch {
	case errors.Is(err, session.ErrSessionActive),
		errors.Is(err, session.ErrSessionNotOpen),
		errors.Is(err, session.ErrAlreadyJoined),
		errors.Is(err, session.ErrSessionFull),
		errors.Is(err, session.ErrSessionNotDrawn),
		errors.Is(err, session.ErrNotParticipant),
		errors.Is(err, session.ErrInvalidWinner):
		return true
	default:
		return false
	}
}
