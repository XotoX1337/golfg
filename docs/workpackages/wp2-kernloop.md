# Arbeitspaket 2 — Kern-Loop (Sessions & Team-Würfeln)

> **Prompt für eine eigene Claude-Code-Session.** Lies zuerst `docs/PLAN.md` (Abschnitte 1, 4, 5).
> Das ist das Herzstück — sauber und gut getestet umsetzen.

## Ziel
Der vollständige Kickern-Loop in der App (noch ohne Teams-Benachrichtigung): Session starten,
beitreten, verlassen, Live-Anzeige der Teilnehmer, automatisches Team-Würfeln bei Erreichen der
benötigten Spielerzahl.

## Voraussetzungen
WP0 (Struktur, Store) und WP1 (eingeloggter User verfügbar).

## Aufgaben
1. **Session-Manager** (`internal/session`):
   - Eine aktive Session zur Zeit (siehe `docs/PLAN.md` Abschnitt 5).
   - Aktionen: `Start` (durch User, liest `Activity.requiredPlayers`), `Join`, `Leave`.
   - States `OPEN → FULL/DRAWN → DONE/CANCELLED` korrekt verwalten; Doppel-Beitritt verhindern.
   - **Team-Würfeln**: bei Erreichen von `requiredPlayers` Teilnehmer per `drawStrategy=random`
     auf `teamSize`-Teams verteilen (für Tischfußball: 2 Teams à 2). Logik testbar isolieren.
2. **Persistenz**: Sessions/Participations in SQLite, States und Team-Zuordnung speichern.
3. **UI / Handler** (`internal/server`, `web/templates`):
   - Startseite zeigt aktuelle Session-Lage: keine offen → "Ich will kickern"-Button;
     offen → Teilnehmerliste + Join/Leave; gewürfelt → Team-Aufstellung.
   - **Live-Updates** via htmx-Polling (`hx-trigger="every 3s"`), nur das Session-Fragment neu laden.
4. **Unit-Tests** für die Würfel-Logik (gleichmäßige, vollständige, zufällige Verteilung) und die
   State-Übergänge.

## Akzeptanzkriterien
- Kompletter Durchlauf im Browser: User A startet → B, C, D treten bei → bei 4 werden 2v2-Teams
  angezeigt; Session ist geschlossen.
- Verlassen vor Voll-Sein reduziert den Counter korrekt.
- Andere Browser sehen Änderungen innerhalb des Poll-Intervalls.
- `go test ./internal/session/...` grün; Würfel-Logik deterministisch testbar (seedbarer RNG).

## Hinweise / Fallen
- Spielerzahl/Team-Größe **aus der Activity lesen**, nicht `4`/`2` hartcodieren (die "Naht" aus Plan 4).
- RNG injizierbar gestalten, damit Tests deterministisch sind.
- Race-Conditions bei gleichzeitigem Join beachten (Transaktion/Lock um "4. Spieler tritt bei → würfeln").
