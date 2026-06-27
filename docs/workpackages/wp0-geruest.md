# Arbeitspaket 0 — Gerüst

> **Prompt für eine eigene Claude-Code-Session.** Lies zuerst `docs/PLAN.md` (Abschnitte 2, 3, 6, 7)
> vollständig, bevor du anfängst. Halte dich an die dortigen Grundprinzipien (public Repo, keine
> Secrets, alles konfigurierbar).

## Ziel
Das Projekt auf die Zielstruktur umstellen und ein lauffähiges, leeres Gerüst herstellen: konfigurierbar
per Datei + ENV, mit SQLite und Migrationen, als schlichter Foreground-Server (für Container-Deployment).
Noch keine Auth, keine Teams, keine Geschäftslogik.

## Voraussetzungen
Keine (erstes Paket). Aktueller Stand: Fiber + viper + zap.

## Aufgaben
1. **Struktur umbauen** gemäß `docs/PLAN.md` Abschnitt 6:
   - Entrypoint nach `cmd/golfg/main.go` verschieben (Graceful-Shutdown via SIGINT/SIGTERM).
   - Bestehende Handler/Views nach `internal/server` bzw. `web/templates`, `web/static` überführen.
   - `//go:embed` für `web/templates` und `web/static` beibehalten (Single-Binary).
2. **Config erweitern** (`internal/config`):
   - Struct gemäß `golfg.example.toml` (Abschnitt 7) mit Sektionen `app`, `auth`, `teams`, `session`.
   - viper: Datei **und** ENV-Override (`GOLFG_<SECTION>_<KEY>`), sinnvolle Defaults.
   - **Hartcodierte Bind-Adresse `192.168.38.131` entfernen** → `app.host` + `app.port`.
   - `golfg.example.toml` mit Platzhaltern + Kommentaren anlegen; echte `golfg.toml` und `.env`
     in `.gitignore` aufnehmen.
3. **SQLite + Migrationen** (`internal/store`):
   - SQLite-Treiber einbinden (z.B. `modernc.org/sqlite`, CGo-frei → einfacheres Cross-Compile).
   - Einfacher Migrations-Mechanismus (eingebettete SQL-Dateien oder Code). Schema für
     `users`, `activities`, `sessions`, `participations` gemäß Domänenmodell (Abschnitt 4) anlegen.
   - **Seed** `activities` mit `Tischfußball` (requiredPlayers=4, teamSize=2, drawStrategy=random).
4. **Smoke-Test-Route** `/` rendert weiterhin eine simple Seite, Server bindet an konfiguriertes host:port.

## Akzeptanzkriterien
- `make run` startet den Server an konfiguriertem Host/Port (Default `0.0.0.0:9000`), keine hartcodierte IP.
- `go build ./...` und `make compile` (Cross-Compile) laufen fehlerfrei.
- Frischer Start ohne Config-Datei erzeugt sinnvolle Defaults; ENV-Overrides greifen.
- SQLite-DB wird angelegt, Migrationen laufen, `Tischfußball` ist geseedet.
- `golfg.toml`/`.env` sind gitignored; `golfg.example.toml` ist eingecheckt.

## Hinweise / Fallen
- `modernc.org/sqlite` vermeidet CGo → behält den einfachen Cross-Compile aus dem `Makefile`.
- Embed-Pfade ändern sich durch den Umzug — Template-Render-Pfade entsprechend anpassen.
- Keine echten Secrets committen; nur Platzhalter in `golfg.example.toml`.
