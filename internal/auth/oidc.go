package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

// initOIDC discovers the tenant's OIDC configuration and builds the oauth2
// client. The issuer embeds the tenant id, so tokens are validated against this
// specific Entra tenant.
func (m *Manager) initOIDC(ctx context.Context) error {
	issuer := fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", m.cfg.Auth.TenantID)
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return fmt.Errorf("discover provider: %w", err)
	}
	m.verifier = provider.Verifier(&oidc.Config{ClientID: m.cfg.Auth.ClientID})
	m.oauth = &oauth2.Config{
		ClientID:     m.cfg.Auth.ClientID,
		ClientSecret: m.cfg.Auth.ClientSecret,
		RedirectURL:  m.redirectURL(),
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	return nil
}

// redirectURL is app.base_url + /auth/callback and must match the redirect URI
// registered in Entra exactly.
func (m *Manager) redirectURL() string {
	return strings.TrimRight(m.cfg.App.BaseURL, "/") + "/auth/callback"
}

// startSSO generates state+nonce, stashes them in the session, and redirects the
// browser to Entra's authorization endpoint.
func (m *Manager) startSSO(c *fiber.Ctx) error {
	state, err := randomToken()
	if err != nil {
		return err
	}
	nonce, err := randomToken()
	if err != nil {
		return err
	}

	sess, err := m.sessions.Get(c)
	if err != nil {
		return err
	}
	sess.Set(oidcStateKey, state)
	sess.Set(oidcNonceKey, nonce)
	if err := sess.Save(); err != nil {
		return err
	}

	return c.Redirect(m.oauth.AuthCodeURL(state, oidc.Nonce(nonce)), fiber.StatusFound)
}

// handleCallback completes the authorization-code flow: verify state, exchange
// the code, validate the ID token (and nonce), cache the user and log them in.
func (m *Manager) handleCallback(c *fiber.Ctx) error {
	if !m.cfg.AuthEnabled() {
		return c.Redirect("/auth/login", fiber.StatusFound)
	}

	sess, err := m.sessions.Get(c)
	if err != nil {
		return err
	}
	wantState, _ := sess.Get(oidcStateKey).(string)
	wantNonce, _ := sess.Get(oidcNonceKey).(string)
	// One-shot values: drop them regardless of outcome.
	sess.Delete(oidcStateKey)
	sess.Delete(oidcNonceKey)

	if wantState == "" || c.Query("state") != wantState {
		return fiber.NewError(fiber.StatusBadRequest, "invalid state")
	}
	if errParam := c.Query("error"); errParam != "" {
		m.logger.Warn("oidc callback error", zap.String("error", errParam),
			zap.String("description", c.Query("error_description")))
		return fiber.NewError(fiber.StatusUnauthorized, "login was denied")
	}

	ctx := c.UserContext()
	token, err := m.oauth.Exchange(ctx, c.Query("code"))
	if err != nil {
		m.logger.Warn("oidc token exchange", zap.Error(err))
		return fiber.NewError(fiber.StatusBadGateway, "token exchange failed")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return fiber.NewError(fiber.StatusBadGateway, "no id_token in response")
	}
	idToken, err := m.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		m.logger.Warn("oidc verify", zap.Error(err))
		return fiber.NewError(fiber.StatusUnauthorized, "invalid id token")
	}
	if idToken.Nonce != wantNonce {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid nonce")
	}

	var claims struct {
		OID               string `json:"oid"`
		Sub               string `json:"sub"`
		Name              string `json:"name"`
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "cannot read claims")
	}

	oid := claims.OID
	if oid == "" {
		oid = claims.Sub // fall back to subject if the oid claim is absent
	}
	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}
	name := claims.Name
	if name == "" {
		name = email
	}

	u, err := m.users.UpsertByEntraOID(oid, name, email)
	if err != nil {
		m.logger.Error("cache sso user", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "could not store user")
	}
	if err := m.login(c, u); err != nil {
		return err
	}
	m.logger.Info("user logged in via SSO", zap.String("user", u.DisplayName))
	return c.Redirect("/", fiber.StatusFound)
}
