# Feature — Warteschlange / nächste Runde

> **Prompt für eine eigene Claude-Code-Session.** Lies zuerst `docs/PLAN.md` (Abschnitt 4, States)
> und `internal/session/manager.go` (`Finish`, `ReRoll`, `drawTeams`). Dockt am `Finish`-Flow an.
> War als Aufgabe 4 in `wp4-feinschliff.md` vorgesehen, wurde dort bewusst herausgelöst.

## Ziel
Während eine Partie läuft (Tisch belegt, Session `DRAWN`), können sich Nutzer **die nicht der
aktiven Session angehören** für die nächste Runde vormerken. Sobald die laufende Partie via
`Manager.Finish` auf `DONE` geht, wird aus der Warteschlange automatisch die nächste Runde eröffnet.

## Voraussetzungen
WP0–WP3 sowie WP4-Feinschliff (Auto-Expire, Re-Roll, Historie) abgeschlossen — letzteres liefert den
`Finish`-Flow, an dem dieses Feature ansetzt. State-Maschine: `OPEN → DRAWN → DONE`.

## Aufgaben
1. **Datenmodell**: Warteschlange persistieren (neue Tabelle `queue` oder Wiederverwendung von
   `participations` mit eigenem Status). Migration in `internal/store/migrations/` ergänzen
   (filename-geordnet, in `schema_migrations` getrackt). Pro Activity genau **eine** Warteschlange.
2. **Vormerken/Austragen**: Endpoints + UI-Buttons, um sich an-/abzumelden, solange eine Session
   `DRAWN` ist. Wer in der aktiven Session spielt, darf sich **nicht** zusätzlich vormerken.
3. **Übergabe bei `Finish`**: in `Manager.Finish` nach dem `DONE`-Übergang prüfen, ob eine
   Warteschlange existiert. Falls ja, eine neue `OPEN`-Session anlegen und die Wartenden (FIFO)
   übernehmen. Ist die nächste Runde damit schon voll (`RequiredPlayers` erreicht), direkt würfeln
   (`drawTeams` / vorhandener Draw-Pfad) und `TeamsDrawn` notifizieren.
4. **Lobby-UI**: Warteschlange anzeigen (Wartende, Position, "n vorgemerkt"), htmx-Live-Update wie
   die übrige Lobby. Zustand "nächste Runde startet" sauber kommunizieren.
5. **Teams-Notify** (optional, sparsam): Hinweis, wenn die nächste Runde aus der Warteschlange
   eröffnet wurde, über den vorhandenen `session.Notifier`.

## Akzeptanzkriterien
- Bei laufender (`DRAWN`) Session können sich Nicht-Teilnehmer vormerken und wieder austragen.
- Nach `Finish` wird die nächste Runde aus der Warteschlange in FIFO-Reihenfolge eröffnet.
- Wird die nächste Runde dabei voll, würfelt sie automatisch und postet (falls konfiguriert) Teams.
- Migration läuft sauber idempotent; bestehende Daten bleiben unberührt.

## Hinweise / Fallen
- `Manager` serialisiert über `m.mu` — Queue-Operationen unter demselben Lock halten, keine
  geschachtelten Locks (siehe `ReRoll`/`Finish`).
- FIFO-Reihenfolge stabil halten (Reihenfolge der Vormerkung, nicht Map-Iteration).
- Race vermeiden: zwischen `DONE` und Eröffnung der Folge-Session darf keine zweite Session am
  selben Tisch entstehen — Übergabe atomar im selben kritischen Abschnitt.
- Auto-Expire (Reaper) beachten: eine frisch eröffnete, aber unvollständige Folge-Session unterliegt
  ebenfalls dem Expiry — das ist gewollt, aber bewusst entscheiden.
- Kein Over-Engineering: eine Warteschlange pro Activity reicht.
