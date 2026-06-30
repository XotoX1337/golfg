package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/XotoX1337/golfg/web"
	"github.com/gofiber/template/html/v2"
)

// TestAvatarPartialRendersPhotoBranch renders the "avatar" partial through the
// real template engine with HasPhoto=true, ensuring the <img> branch executes
// (template errors only surface at render time, not at go build).
func TestAvatarPartialRendersPhotoBranch(t *testing.T) {
	tmplFS, err := fs.Sub(web.Templates, "templates")
	if err != nil {
		t.Fatal(err)
	}
	engine := html.NewFileSystem(http.FS(tmplFS), ".html")
	// All funcs referenced anywhere in the template set must exist before Load.
	engine.AddFunc("Initials", initials)
	engine.AddFunc("dict", dict)
	engine.AddFunc("OpenSlots", openSlots)
	engine.AddFunc("Name", func() string { return "go LFG" })
	engine.AddFunc("Accent", func() string { return "#000" })
	engine.AddFunc("PlayCTA", func() string { return "" })
	engine.AddFunc("Version", func() string { return "test" })
	engine.AddFunc("Year", func() int { return 2026 })
	engine.AddFunc("DateTime", func(time.Time) string { return "" })
	if err := engine.Load(); err != nil {
		t.Fatalf("load templates: %v", err)
	}

	var buf bytes.Buffer
	data := map[string]any{"ID": "u1", "Name": "Anton Berg", "HasPhoto": true, "Class": "avatar-sm"}
	if err := engine.Render(&buf, "avatar", data); err != nil {
		t.Fatalf("render avatar: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `src="/avatar/u1"`) {
		t.Errorf("photo branch should emit the /avatar/u1 img src, got:\n%s", out)
	}
	if !strings.Contains(out, "AB") {
		t.Errorf("photo branch should still carry the initials fallback, got:\n%s", out)
	}
	if !strings.Contains(out, "avatar-sm") {
		t.Errorf("photo branch should apply the passed Class, got:\n%s", out)
	}
}
