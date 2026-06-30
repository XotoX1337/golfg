package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/XotoX1337/golfg/internal/user"
)

// graphPhotoURL is the Microsoft Graph endpoint for the signed-in user's own
// profile photo. We only ever read /me — never other users — so the login keeps
// the user-consentable User.Read scope (no admin consent, no User.ReadBasic.All).
const graphPhotoURL = "https://graph.microsoft.com/v1.0/me/photo/$value"

// graphTimeout bounds the photo call so a slow Graph endpoint can never hold up
// the login redirect. maxPhotoBytes caps how much we read defensively.
const (
	graphTimeout  = 5 * time.Second
	maxPhotoBytes = 4 << 20 // 4 MiB
)

// errNoPhoto signals the common, non-error case: the user simply has no photo
// set (Graph answers 404). Treated as "no avatar", not a failure.
var errNoPhoto = errors.New("user has no profile photo")

// fetchAndStorePhoto pulls the user's M365 photo and caches it. It is strictly
// best-effort: any failure (no photo, timeout, revoked permission) is logged and
// swallowed so login is never affected.
func (m *Manager) fetchAndStorePhoto(ctx context.Context, u *user.User, accessToken string) {
	if accessToken == "" {
		return
	}
	photo, etag, err := fetchGraphPhoto(ctx, accessToken)
	if err != nil {
		// No-photo is the expected default for many accounts: keep it quiet (Debug).
		if errors.Is(err, errNoPhoto) {
			m.logger.Debug("no M365 photo for user", zap.String("user", u.DisplayName))
		} else {
			m.logger.Warn("fetch M365 photo", zap.String("user", u.DisplayName), zap.Error(err))
		}
		return
	}
	if err := m.users.SetPhoto(u.ID, photo, etag); err != nil {
		m.logger.Warn("cache M365 photo", zap.String("user", u.DisplayName), zap.Error(err))
	}
}

// fetchGraphPhoto GETs /me/photo/$value with the Graph access token and returns
// the image bytes plus a content-hash etag (used to skip no-op rewrites and as
// the /avatar HTTP ETag). A 404 maps to errNoPhoto.
func fetchGraphPhoto(ctx context.Context, accessToken string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, graphTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, graphPhotoURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, "", errNoPhoto
	default:
		return nil, "", fmt.Errorf("graph photo: unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPhotoBytes))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", errNoPhoto
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]), nil
}
