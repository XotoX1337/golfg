# Arbeitspaket 3 — Teams-Benachrichtigung

> **Prompt für eine eigene Claude-Code-Session.** Lies zuerst `docs/PLAN.md` (Abschnitte 1, 2, 8).
> Teams ist **nur Benachrichtigungs-Medium** (Outbound-Webhook), keine Interaktion in Teams.

## Ziel
Bei den richtigen Ereignissen eine Nachricht in den "Kickern"-Teams-Kanal posten, mit Deep-Links
zurück in die App — plus eine verständliche Setup-Anleitung. Ohne konfigurierten Webhook werden
Posts nur geloggt (Graceful Degradation).

## Voraussetzungen
WP0 (Config `[teams]`, `app.base_url`) und WP2 (Session-Events vorhanden).

## Aufgaben
1. **Teams-Client** (`internal/teams`):
   - Outbound-POST an `teams.webhook_url` (Power-Automate-Workflow-URL) mit **Adaptive Card** JSON.
   - Wenn `webhook_url` leer: Nachricht nur via zap loggen (kein Fehler).
   - Robust: Timeout, Fehler loggen statt App crashen lassen.
2. **Events verdrahten** (sparsam, siehe Plan Abschnitt 5):
   - **Session-Start:** *"⚽ {Name} will kickern – noch {n} frei"* + Button/Deep-Link
     (`app.base_url` → offene Session).
   - **Teams gewürfelt:** *"Es geht los! Team A: … — Team B: …"*.
   - (Optional) "noch 1 frei".
3. **Deep-Links**: Buttons/Links in den Cards zeigen auf die App (`app.base_url`), führen zur Session.
4. **Doku** `docs/setup-teams.md` schreiben: Schritt-für-Schritt
   - Teams-Kanal "Kickern" anlegen
   - Power-Automate-Workflow "When a Teams webhook request is received" erstellen
   - Webhook-URL kopieren und in Config/ENV (`GOLFG_TEAMS_WEBHOOK_URL`) eintragen
   - Hinweis auf O365-Connector-Deprecation (Workflows ist der Nachfolger).

## Akzeptanzkriterien
- Mit konfiguriertem Webhook erscheinen bei Start und Würfeln Nachrichten im Kanal; Deep-Links
  führen zurück in die App.
- Ohne Webhook läuft alles, Posts werden geloggt.
- `docs/setup-teams.md` ist vollständig und ohne Vorwissen nachvollziehbar.

## Hinweise / Fallen
- Power-Automate-Workflow erwartet ein bestimmtes Payload-/Adaptive-Card-Format — Format in der Doku
  und im Code dokumentieren.
- Keine Webhook-URL committen (enthält ein Secret-Token) — nur Platzhalter in `golfg.example.toml`.
- Outbound-Call darf den Request-Flow nicht blockieren (async/goroutine + Logging).
