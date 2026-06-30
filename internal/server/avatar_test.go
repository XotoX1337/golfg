package server

import (
	"io"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/XotoX1337/golfg/internal/store"
	"github.com/XotoX1337/golfg/internal/user"
)

// jpegBytes is a minimal byte sequence whose magic number makes
// http.DetectContentType report image/jpeg.
var jpegBytes = []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}

func newAvatarTestServer(t *testing.T) (*Server, *user.Repository) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"), zap.NewNop())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	repo := user.NewRepository(st)
	s := &Server{users: repo, app: fiber.New()}
	s.app.Get("/avatar/:userID", s.showAvatar)
	return s, repo
}

func TestShowAvatarNotFound(t *testing.T) {
	s, _ := newAvatarTestServer(t)
	resp, err := s.app.Test(httptest.NewRequest(fiber.MethodGet, "/avatar/nobody", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestShowAvatarServesPhotoWithETag(t *testing.T) {
	s, repo := newAvatarTestServer(t)
	u, err := repo.UpsertDev("Anton", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repo.SetPhoto(u.ID, jpegBytes, "etag-abc"); err != nil {
		t.Fatalf("SetPhoto: %v", err)
	}

	resp, err := s.app.Test(httptest.NewRequest(fiber.MethodGet, "/avatar/"+u.ID, nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	wantETag := `"etag-abc"`
	if et := resp.Header.Get("ETag"); et != wantETag {
		t.Errorf("ETag = %q, want %q", et, wantETag)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != len(jpegBytes) {
		t.Errorf("body length = %d, want %d", len(body), len(jpegBytes))
	}

	// A matching If-None-Match must short-circuit to 304 with no body.
	req := httptest.NewRequest(fiber.MethodGet, "/avatar/"+u.ID, nil)
	req.Header.Set("If-None-Match", wantETag)
	resp304, err := s.app.Test(req)
	if err != nil {
		t.Fatalf("conditional request: %v", err)
	}
	if resp304.StatusCode != fiber.StatusNotModified {
		t.Fatalf("conditional status = %d, want 304", resp304.StatusCode)
	}
}
