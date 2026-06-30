package user

import (
	"bytes"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/XotoX1337/golfg/internal/store"
)

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"), zap.NewNop())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewRepository(st)
}

func TestSetAndGetPhoto(t *testing.T) {
	repo := newTestRepo(t)
	u, err := repo.UpsertDev("Anton", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.HasPhoto {
		t.Fatalf("new user should not have a photo")
	}

	// No photo yet.
	if _, _, ok, err := repo.GetPhoto(u.ID); err != nil || ok {
		t.Fatalf("GetPhoto before set: ok=%v err=%v, want ok=false", ok, err)
	}

	want := []byte{0xff, 0xd8, 0xff, 0x01, 0x02} // pretend-JPEG bytes
	if err := repo.SetPhoto(u.ID, want, "etag-1"); err != nil {
		t.Fatalf("SetPhoto: %v", err)
	}

	got, etag, ok, err := repo.GetPhoto(u.ID)
	if err != nil || !ok {
		t.Fatalf("GetPhoto after set: ok=%v err=%v", ok, err)
	}
	if !bytes.Equal(got, want) || etag != "etag-1" {
		t.Fatalf("GetPhoto = (%v, %q), want (%v, %q)", got, etag, want, "etag-1")
	}

	// HasPhoto flag now reflects the cached photo.
	reloaded, err := repo.GetByID(u.ID)
	if err != nil || reloaded == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !reloaded.HasPhoto {
		t.Fatalf("HasPhoto should be true after caching a photo")
	}
}

func TestSetPhotoReplacesOnNewEtag(t *testing.T) {
	repo := newTestRepo(t)
	u, err := repo.UpsertDev("Berta", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := repo.SetPhoto(u.ID, []byte("old"), "etag-1"); err != nil {
		t.Fatalf("SetPhoto old: %v", err)
	}
	// Same etag: the no-op upsert must leave the stored bytes untouched.
	if err := repo.SetPhoto(u.ID, []byte("ignored"), "etag-1"); err != nil {
		t.Fatalf("SetPhoto same etag: %v", err)
	}
	if got, _, _, _ := repo.GetPhoto(u.ID); string(got) != "old" {
		t.Fatalf("unchanged etag should keep old bytes, got %q", got)
	}
	// New etag: bytes are replaced.
	if err := repo.SetPhoto(u.ID, []byte("new"), "etag-2"); err != nil {
		t.Fatalf("SetPhoto new etag: %v", err)
	}
	got, etag, _, _ := repo.GetPhoto(u.ID)
	if string(got) != "new" || etag != "etag-2" {
		t.Fatalf("GetPhoto = (%q, %q), want (new, etag-2)", got, etag)
	}
}
