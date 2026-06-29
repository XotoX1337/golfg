# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**go LFG** ("GoLang Looking For Group Tool") is a Go web application served by [Fiber v2](https://github.com/gofiber/fiber), rendering server-side HTML templates with [htmx](https://htmx.org) for interactivity. It runs as a plain foreground HTTP server (graceful shutdown on SIGINT/SIGTERM), built to be deployed in a container (Docker/LXD).

## Conventions

- **Language**: chat may be German or English (either is fine). Everything that ships or is shared is **English** — code, comments, identifiers, commit messages, PR/issue text on GitHub, and the `docs/` content.

## Commands

```bash
make run       # go run ./cmd/golfg (local dev)
make build     # go mod tidy + build ./cmd/golfg into dist/ for the host platform
make compile   # cross-compile linux-arm, linux-amd64, windows-amd64, darwin-amd64 into dist/
make all       # clean + compile
make esbuild   # bundle/minify assets/css/style.css -> web/static/css/app.min.css (requires esbuild on PATH)
make icon      # regenerate web/static/img/favicon.ico from assets/img/favicon.svg
```

Run all tests with `go test ./...`, or a single test with `go test ./path/to/pkg -run TestName`.

Override the listen port at runtime: `go run ./cmd/golfg --port 9001` (or `-p`). Override anything else via `GOLFG_<SECTION>_<KEY>` env vars (e.g. `GOLFG_APP_HOST`, `GOLFG_TEAMS_WEBHOOK_URL`).

The restructure plan and per-step work packages live under `docs/` (`docs/PLAN.md`, `docs/workpackages/`). Consult them before larger changes.

## Architecture

- **`cmd/golfg/main.go`** — entrypoint. Parses flags, builds the zap logger (stdout + `golfg.log`), loads config, opens the store, builds the server, runs `srv.Listen()` in a goroutine, and blocks on a SIGINT/SIGTERM channel; on signal it calls `srv.Shutdown()` (store/logger closed via `defer`). Data files (config, log, DB) resolve next to the executable.
- **`internal/config`** — viper-based config. `config.Load(dataDir, logger)` reads `golfg.toml` (searched in the data dir and CWD), applies defaults, and binds `GOLFG_<SECTION>_<KEY>` env overrides (ENV wins over file). Writes a default `golfg.toml` if none exists. Sections: `app` (host/port/base_url), `auth`, `teams`, `session`, `branding`. App metadata (name/version/service name) are consts here. `AuthEnabled()`/`TeamsEnabled()` gate graceful degradation. White-label accessors (`AppName()`, `AccentColor()`, `PlayCTA()`, `PlayAnnouncement()`) return the configured `branding.*` literal or fall back: name/color to a default const, the play wording to `""` so the template/Teams layer falls back to the localized string. New branding keys also need an `envBindings` entry.
- **`internal/store`** — SQLite via `modernc.org/sqlite` (pure Go, no CGo → cross-compile stays simple). `store.Open` runs embedded migrations from `internal/store/migrations/*.sql` (tracked in `schema_migrations`, applied in filename order, each in a tx) and the data is seeded by a migration. Schema: `users`, `activities`, `sessions`, `participations`.
- **`internal/server`** — Fiber setup. `server.New` builds the HTML template engine over the embedded templates and registers routes; one handler per file (e.g. `index.go`). Render paths are relative to `web/templates` (the prefix is stripped via `fs.Sub`), e.g. `c.Render("index/show", ...)`. The `withLocale` middleware (`i18n.go`) runs first and binds the request translator.
- **`internal/i18n`** — UI translations via [go-i18n](https://github.com/nicksnyder/go-i18n). TOML message catalogs live in `internal/i18n/locales/<lang>.toml` (embedded), one flat `key = "value"` per string; placeholders use Go template syntax (`{{.Name}}`). English is the default/fallback; missing keys fall back to English, then to the key itself. The `withLocale` middleware resolves the language (`?lang=` → `golfg_lang` cookie → `Accept-Language`) and binds `T`/`Lang` for templates — call as `{{ call .T "key" "Name" .Foo }}` (`$.T` under `with`/`range`), with `<html lang="{{ .Lang }}">`. Handlers pass error/message **keys**, not literal text (e.g. `auth/dev.go`), so translation stays in the template layer.
- **`web/web.go`** — `//go:embed` for `web/templates` and `web/static` (single binary). `base.html` defines reusable `header`/`footer` blocks; feature views compose them with `{{ template "header" . }}`.
- **`assets/`** (source) vs **`web/static/`** (built/served). CSS in `assets/css/` is bundled by esbuild into `web/static/css/app.min.css`; `web/static/` is what gets embedded and served. Edit sources in `assets/`, then run `make esbuild`.

## Gotchas

- Static assets are served from the **embedded** `web/static` FS, so asset changes require a rebuild (and esbuild re-bundle) to take effect — not just an edit under `assets/`.
- Config, log and DB files (`golfg.toml`, `golfg.log`, `golfg.db`) resolve relative to the **executable's** directory. Under `go run` that's a temp build dir; `golfg.toml` is also searched in the CWD, so a repo-root `golfg.toml` works for local dev (DB/log still land in the temp dir).
- Real config is gitignored (`golfg.toml`, `.env`, `golfg.db`, `golfg.log`); only `golfg.example.toml` is committed. Never commit secrets — inject them via ENV.
- The old `/config` and `/config/reload` JSON endpoints were removed (they would have leaked secrets); config is no longer exposed over HTTP.
