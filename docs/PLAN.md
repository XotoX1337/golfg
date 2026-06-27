# go LFG — Restructure-Plan

> **Kernidee:** Ein Tool, um im Büro spontan Kollegen zum **Kickern** (Tischfußball) zu finden.
> Einer startet, andere machen mit, bei 4 Spielern werden die Teams gewürfelt und alle via
> Microsoft Teams benachrichtigt. Diese Datei ist die zentrale Referenz für die Umsetzung.

## 1. Produktvision & MVP-Flow

1. User öffnet die App im Intranet → via **Entra-SSO** automatisch erkannt (Name/Email aus M365).
2. User startet eine **Session** ("Ich will kickern") → App postet in den **"Kickern"-Teams-Kanal**:
   *"⚽ Anton will kickern – noch 3 frei → [Mitmachen]"* (Deep-Link in die App).
3. Kollegen öffnen die App, sehen die offene Session **live** und treten bei.
4. Bei **4 Spielern**: App **würfelt 2v2**, schließt die Session und postet das Ergebnis in den Kanal.
5. **Teams ist reines Benachrichtigungs-Medium** — die gesamte Interaktion passiert in der App.

## 2. Grundprinzipien für das öffentliche Repo

Das Repo bleibt **public auf GitHub**. Daraus folgt verbindlich:

- **Keine Secrets im Repo.** Echte Config (`golfg.toml`, `.env`) ist `.gitignore`d.
- **Alles Firmenspezifische ist konfigurierbar** (Tenant-/Client-IDs, Secrets, Webhook-URL, Host/Port,
  Texte/Branding). Nichts wird hartcodiert — insbesondere wird die aktuell hartcodierte Bind-Adresse
  `192.168.38.131` aus `main.go` entfernt.
- **Config per Datei *und* Umgebungsvariablen** (viper env-binding), damit Secrets auf dem Server
  per ENV injiziert werden können, ohne eine Datei abzulegen.
- **`golfg.example.toml`** mit Platzhaltern + Kommentaren wird mitgeliefert.
- **Verständliche Setup-Anleitungen** liegen unter `docs/`:
  - `docs/setup-entra.md` — Schritt-für-Schritt Entra-App-Registrierung (SSO)
  - `docs/setup-teams.md` — Schritt-für-Schritt Power-Automate-Workflow für den Kickern-Kanal
- App muss **ohne** konfigurierte Teams-/Entra-Anbindung lauffähig sein (Graceful Degradation:
  z.B. Teams-Posts werden nur geloggt, Auth optional im Dev-Modus) — damit Fremde das Projekt
  ausprobieren können.

## 3. Tech-Stack

| Bereich | Wahl | Status |
|---|---|---|
| Sprache/Web | Go + Fiber v2 | Bestand |
| UI | htmx + Go-Templates | Bestand |
| Live-Updates | htmx-Polling (`hx-trigger="every 3s"`) | neu (SSE als späteres Upgrade) |
| Persistenz | SQLite | neu |
| Auth | Entra ID / OIDC (`coreos/go-oidc` + `golang.org/x/oauth2`) | neu |
| Teams | Power-Automate-Workflow-Webhook (Outbound-POST, Adaptive Card) | neu |
| Deployment | Docker/LXD-Container, schlichter Foreground-Server (SIGTERM-Graceful-Shutdown) | geändert (kardianos/service entfernt) |
| Config | viper/TOML + ENV | Bestand, erweitert |
| Logging | zap | Bestand |

**Warum diese Architektur:** Intranet-Hosting erlaubt SSO (Browser-Redirect) und Outbound-Webhooks,
aber **keine** eingehenden Bot-Callbacks von Microsoft — deshalb findet alle Interaktion in der App
statt und Teams dient nur als Benachrichtigungskanal mit Deep-Links.

## 4. Domänenmodell

- **Activity** — Spiel-Definition. Seed: `Tischfußball` (`requiredPlayers=4`, `teamSize=2`,
  `drawStrategy=random`). Diese Abstraktion ist die "Naht" für spätere Spiele (Darts, MarioKart …),
  ohne dass jetzt generisch gebaut wird.
- **User** — `id, entraOID, displayName, email`. Aus SSO gecached.
- **Session** — `id, activityID, creatorID, status, createdAt, expiresAt`.
- **Participation** — `sessionID, userID, joinedAt, team (A/B nach Würfeln)`.

## 5. Session-Lifecycle & Edge-Cases

- **States:** `OPEN → FULL/DRAWN → (DONE | CANCELLED | EXPIRED)`
- **Eine aktive Session** zur Zeit (ein Tisch). Multi-Session später trivial erweiterbar.
- **Verlassen:** User kann vor dem Voll-Sein wieder austreten (Counter sinkt).
- **Auto-Expire:** offene Session ohne 4 Spieler nach `session.expire_minutes` → `EXPIRED`.
- **Teams-Posts sparsam:** nur bei **Start** und **Teams-gewürfelt** (Beitritte sieht man live in der App).
- **Re-Roll** der Teams: nice-to-have (Phase 4).

## 5a. UI/UX & Screens

Es ist eine **kleine, fokussierte Web-App** — im Kern eine einzige lebende Seite. Trotzdem soll sie
sich aufgeräumt und bewusst gestaltet anfühlen, nicht nach Default-Template.

**Charakter & Stack**
- **Mobile-first**: Leute checken das am Handy zwischen Tür und Angel. Touch-freundliche Buttons.
- **Stack aus dem Bestand**: Pico CSS (`assets/css/pico.min.css`), Remixicon, htmx. Semantisches,
  klassenarmes HTML mit minimalem Custom-CSS (`assets/css/style.css`).
- **Branding konfigurierbar** (public Repo): App-Name/Titel und Akzentfarbe über Config setzbar,
  damit Fremde es eindecken können. Keine firmenspezifischen Texte hartcodiert.
- **Light/Dark** via Picos `data-theme` (aktuell fix `light` in `base.html` → umschaltbar machen).

**Screens**
1. **Lobby (Hauptseite)** — zustandsabhängig, per htmx-Polling live aktualisiert:
   - *Keine Session offen* → großer CTA "⚽ Ich will kickern".
   - *Session offen* → Teilnehmerliste/Avatare, Fortschritt "3 / 4", Join-/Leave-Button.
   - *Teams gewürfelt* → Aufstellung Team A vs. Team B, klar visuell getrennt.
2. **Login / Dev-Login** — schlicht; im Dev-Modus klar als solcher gekennzeichnet.
3. **Historie/Stats** (später, WP4) — Liste vergangener Sessions.

**Komponenten/States**
- Wiederverwendbares **Session-Fragment** (für htmx-Teil-Reload), Teilnehmer-Liste, Fortschrittsanzeige,
  Team-Aufstellung, Header mit aktuellem User + Logout.
- **Empty/Loading/Error-States** bewusst gestalten (z.B. "Gerade will keiner kickern 😴").
- Dezente Übergänge beim Live-Update, damit Polling nicht "springt".

> Für eine bewusste visuelle Richtung (Typo, Farbe, Layout) kann beim Bauen das `frontend-design`-Skill
> herangezogen werden.

## 6. Zielstruktur

```
cmd/golfg/main.go          # Entrypoint + Graceful-Shutdown (Signal-Handling)
internal/
  config/                  # viper-Laden (Datei + ENV), Struct, Defaults
  auth/                    # Entra OIDC: Login, Callback, Session-Cookie, Middleware
  server/                  # Fiber-Setup, Routing, Template-Engine
  activity/                # Spiel-Definitionen (+ Seed)
  session/                 # Session-Manager, Team-Würfel-Logik (Kern!)
  user/                    # Identität aus SSO
  teams/                   # Webhook-Client (Adaptive Card)
  store/                   # SQLite + Migrationen
web/
  templates/               # //go:embed
  static/                  # //go:embed (Assets-Build via esbuild)
docs/
  PLAN.md  setup-entra.md  setup-teams.md
golfg.example.toml
```

Embed bleibt → weiterhin ein einzelnes Binary.

## 7. Konfiguration (`golfg.example.toml`)

```toml
[app]
host = "0.0.0.0"          # frei konfigurierbar (ersetzt hartcodierte IP)
port = 9000
base_url = "https://kicker.intranet"   # für Deep-Links in Teams

[auth]                    # leer lassen = Dev-Modus ohne SSO
tenant_id = ""
client_id = ""
client_secret = ""        # besser via ENV: GOLFG_AUTH_CLIENT_SECRET

[teams]                   # leer lassen = Posts werden nur geloggt
webhook_url = ""          # besser via ENV: GOLFG_TEAMS_WEBHOOK_URL

[session]
expire_minutes = 30
```

ENV-Override-Schema: `GOLFG_<SECTION>_<KEY>` (z.B. `GOLFG_TEAMS_WEBHOOK_URL`).

## 8. Externes Setup (außerhalb des Codes)

- **Entra App-Registrierung:** Redirect-URI eintragen, Client-Secret erzeugen, Scopes
  `openid profile email`. → Anleitung: `docs/setup-entra.md`
- **Teams-Kanal "Kickern" + Power-Automate-Workflow** ("When a Teams webhook request is received"),
  Webhook-URL kopieren. → Anleitung: `docs/setup-teams.md`

## 9. Roadmap / Arbeitspakete

Jedes Arbeitspaket ist ein eigenständiger Prompt unter `docs/workpackages/` und kann in einer
separaten Session abgearbeitet werden. Reihenfolge einhalten (Abhängigkeiten).

| WP | Inhalt | Datei |
|----|--------|-------|
| 0 | Gerüst: neue Struktur, Config (Datei+ENV), Bind-Adresse fixen, SQLite + Migrationen | `docs/workpackages/wp0-geruest.md` |
| 1 | Auth: Entra-OIDC-Login, Session-Cookie, Middleware + `docs/setup-entra.md` | `docs/workpackages/wp1-auth.md` |
| 2 | Kern-Loop: Session erstellen/beitreten/verlassen, Live-Liste, Team-Würfeln bei 4 | `docs/workpackages/wp2-kernloop.md` |
| 3 | Teams: Webhook-Client, Posts bei Start & Würfeln, Deep-Links + `docs/setup-teams.md` | `docs/workpackages/wp3-teams.md` |
| 4 | Feinschliff: Auto-Expire, Re-Roll, simple Historie/Stats | `docs/workpackages/wp4-feinschliff.md` |
| 5 | UI/UX-Politur: Design-Richtung, Branding-Config, Light/Dark, States (nach WP2) | `docs/workpackages/wp5-ui.md` |

> **Hinweis zur UI:** Die *funktionale* UI entsteht bereits in WP2 (damit der Loop bedienbar ist).
> WP5 ist die gezielte gestalterische Politur und kann direkt nach WP2 oder am Ende erfolgen.
