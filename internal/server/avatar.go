package server

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// showAvatar streams a user's cached M365 profile photo. It sits behind
// RequireAuth — photos are never exposed on a public endpoint — and uses the
// cached content-hash as a strong ETag so browsers revalidate cheaply (304 on a
// matching If-None-Match). Users without a cached photo get a 404, which the
// templates render as the initials/icon fallback.
func (s *Server) showAvatar(c *fiber.Ctx) error {
	photo, etag, ok, err := s.users.GetPhoto(c.Params("userID"))
	if err != nil {
		return err
	}
	if !ok {
		return fiber.ErrNotFound
	}

	tag := `"` + etag + `"`
	// Private: the photo is per-user and only served to authenticated requests,
	// so it must not be cached by shared proxies.
	c.Set("Cache-Control", "private, max-age=300")
	c.Set("ETag", tag)
	if c.Get("If-None-Match") == tag {
		return c.SendStatus(fiber.StatusNotModified)
	}

	c.Set(fiber.HeaderContentType, http.DetectContentType(photo))
	return c.Send(photo)
}
