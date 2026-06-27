# Arbeitspaket 1 — Auth (Entra SSO)

> **Prompt für eine eigene Claude-Code-Session.** Lies zuerst `docs/PLAN.md` (Abschnitte 2, 3, 8).
> Beachte: Secrets nie ins Repo; App muss ohne konfigurierte Auth im **Dev-Modus** lauffähig bleiben.

## Ziel
Login via Microsoft Entra ID (OIDC) mit Session-Cookie und Auth-Middleware, plus eine verständliche
Setup-Anleitung. Nutzer werden nach Login mit Name/Email erkannt und in `users` gecached.

## Voraussetzungen
WP0 abgeschlossen (Struktur, Config mit `[auth]`, SQLite mit `users`).

## Aufgaben
1. **OIDC-Flow** (`internal/auth`) mit `coreos/go-oidc` + `golang.org/x/oauth2`:
   - Routen `/auth/login`, `/auth/callback`, `/auth/logout`.
   - Scopes `openid profile email`, State/Nonce-Handling, Token-Validierung.
   - Bei Erfolg: User in `users` upserten (`entraOID`, `displayName`, `email`), **Session-Cookie**
     setzen (signiert, httpOnly, secure).
2. **Auth-Middleware**: geschützte Routen leiten unauthentifizierte User auf `/auth/login`.
3. **Dev-Modus**: Wenn `[auth]` leer ist, kein echtes SSO — stattdessen ein simpler lokaler
   Pseudo-Login (Name eintippen), damit das öffentliche Repo ohne M365 testbar bleibt. Klar im UI
   als Dev-Modus kennzeichnen.
4. **`current user`** in Templates/Context verfügbar machen (Name anzeigen, Logout-Link).
5. **Doku** `docs/setup-entra.md` schreiben: Schritt-für-Schritt Entra-App-Registrierung
   (App registrieren → Redirect-URI `…/auth/callback` → Client-Secret → Scopes → Werte in Config/ENV
   eintragen). Leicht verständlich, mit Hinweis auf benötigte Berechtigungen.

## Akzeptanzkriterien
- Mit konfiguriertem `[auth]`: vollständiger Login-Flow gegen Entra funktioniert, User wird gecached,
  Session-Cookie hält den Login.
- Ohne `[auth]`: Dev-Login funktioniert, App ist nutzbar.
- Geschützte Seiten sind ohne Login nicht erreichbar.
- `docs/setup-entra.md` ist vollständig und für jemanden ohne Vorwissen nachvollziehbar.

## Hinweise / Fallen
- Redirect-URI muss exakt mit `app.base_url` + `/auth/callback` übereinstimmen (Entra ist strikt).
- Im Intranet funktioniert SSO über den **Browser-Redirect** — der Server braucht kein Inbound aus
  dem Internet.
- Cookie `secure` nur über HTTPS; für lokalen Dev ggf. konfigurierbar machen.
