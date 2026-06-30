package user

import (
	"database/sql"
	"fmt"
)

// SetPhoto caches (or refreshes) a user's profile photo. The WHERE clause on the
// upsert makes an unchanged image a no-op: when the new etag matches the stored
// one the row is left untouched, so re-logins don't rewrite the same bytes.
func (r *Repository) SetPhoto(userID string, photo []byte, etag string) error {
	if len(photo) == 0 || etag == "" {
		return fmt.Errorf("photo and etag required")
	}
	_, err := r.db.Exec(`
		INSERT INTO user_photos (user_id, photo, etag, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET
			photo      = excluded.photo,
			etag       = excluded.etag,
			updated_at = CURRENT_TIMESTAMP
		WHERE user_photos.etag <> excluded.etag`,
		userID, photo, etag)
	if err != nil {
		return fmt.Errorf("set photo: %w", err)
	}
	return nil
}

// GetPhoto returns a user's cached photo bytes and etag. ok is false (no error)
// when the user has no cached photo, which the /avatar route turns into a 404.
func (r *Repository) GetPhoto(userID string) (photo []byte, etag string, ok bool, err error) {
	err = r.db.QueryRow(
		`SELECT photo, etag FROM user_photos WHERE user_id = ?`, userID,
	).Scan(&photo, &etag)
	if err == sql.ErrNoRows {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("get photo: %w", err)
	}
	return photo, etag, true, nil
}
