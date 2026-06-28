# Feature — Punkte / Best-of-N beim Abschluss

> **Prompt für eine eigene Claude-Code-Session.** Lies zuerst `docs/PLAN.md` (Abschnitt 5) und
> `internal/session/manager.go` (`Finish`, `showFinishModal`/`finishSession` in `internal/server`).
> War als Aufgabe 5 in `wp4-feinschliff.md` vorgesehen, wurde dort bewusst herausgelöst.

## Ziel
Das Finish-Modal (heute nur "welches Team hat gewonnen?") um eine Ergebnis-Eingabe erweitern:
Modus **best of 1** oder **best of 3**, je Activity konfigurierbar. Das Resultat wird persistiert und
über den `session.Notifier` (Teams) ausgegeben.

## Voraussetzungen
WP0–WP3 sowie WP4-Feinschliff abgeschlossen. Der `Finish`-Flow (`Manager.Finish`,
`server.finishSession`, `web/templates/session` Finish-Modal) existiert und nimmt heute nur das
Gewinner-Team entgegen.

## Aufgaben
1. **Activity-Eigenschaft**: `internal/activity` um einen Modus erweitern (z.B. `BestOf int`,
   Default 1). Migration in `internal/store/migrations/` (Spalte + Seed-Update der bestehenden
   Activities). `activity.Activity`-Struct und Repository-Query mitführen.
2. **Score-Ablage**: Ergebnis je Runde persistieren — entweder neue Spalten auf `sessions`
   (`score_a`, `score_b`) oder eine kleine `scores`-Tabelle, falls feinere Auflösung gewünscht.
   Schema klein halten, kein Over-Engineering.
3. **Finish-Modal erweitern**: bei `BestOf == 1` weiterhin reine Gewinner-Auswahl; bei `BestOf == 3`
   Eingabe der Satz-/Spielstände (Validierung: gültiges Best-of-3-Ergebnis, Gewinner ergibt sich aus
   dem Score). i18n-Keys (en/de) ergänzen, keine literalen Texte in Handlern.
4. **`Manager.Finish` anpassen**: Score validieren, Gewinner-Team ableiten bzw. konsistent prüfen,
   Score speichern. Neue Soft-Fehler (z.B. `ErrInvalidScore`) in `isSoftSessionErr` aufnehmen.
5. **Notify & Historie**: `TeamsDrawn`/Finish-Notification um das Endergebnis ergänzen
   (`session.Notifier`). Score in der Historie-Seite (`/history`, `Manager.History`) anzeigen.

## Akzeptanzkriterien
- Activities tragen einen Best-of-Modus (1 oder 3), per Migration gesetzt; Default bleibt best of 1.
- Best-of-1-Sessions verhalten sich unverändert (nur Gewinner-Team).
- Best-of-3-Sessions verlangen ein gültiges Ergebnis; ungültige Eingaben sind Soft-Fehler ohne Crash.
- Endergebnis steht in der Persistenz, im Teams-Post und in der Historie.

## Hinweise / Fallen
- `Manager.Finish` läuft unter `m.mu`; Validierung dort, nicht im Handler.
- Gewinner-Team und Score müssen konsistent sein — eine Quelle der Wahrheit (Score → Gewinner ableiten).
- Bestehende `WinnerTeam`-Logik und `ErrInvalidWinner` nicht brechen; best of 1 ist der bisherige Pfad.
- i18n: Handler reichen Keys, Übersetzung bleibt in der Template-Schicht.
- Migration idempotent und filename-geordnet; Seed der vorhandenen Activities mitziehen.
