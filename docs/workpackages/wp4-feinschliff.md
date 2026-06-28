# Arbeitspaket 4 — Feinschliff

> **Prompt für eine eigene Claude-Code-Session.** Lies zuerst `docs/PLAN.md` (Abschnitte 4, 5).
> Optionale Verbesserungen, einzeln umsetzbar.

## Ziel
Die Bedienung abrunden: keine Karteileichen-Sessions, Re-Roll-Möglichkeit und eine simple Historie/Stats.

## Voraussetzungen
WP0–WP3 abgeschlossen.

## Aufgaben (einzeln oder gemeinsam umsetzbar)
1. **Auto-Expire**: Hintergrund-Job, der offene Sessions älter als `session.expire_minutes`
   auf `EXPIRED` setzt. UI zeigt abgelaufene Session nicht mehr als aktiv. Optional Teams-Hinweis.
2. **Re-Roll**: Nach dem Würfeln können Teilnehmer die Teams neu auswürfeln (z.B. Button, ggf.
   nur Creator). Ergebnis aktualisiert UI und – falls gewünscht – Teams-Post.
3. **Historie/Stats** (simpel): vergangene Sessions auflisten (wer, wann, welche Teams). Optional
   einfache Zähler ("X-mal gekickert"). Daten liegen bereits in SQLite.
4. **Warteschlange**: während eine Session DRAWN (Tisch belegt) ist, können sich Nutzer **die
   nicht der aktiven Session angehören** für die nächste Runde vormerken. Sobald die laufende
   Partie via `Manager.Finish` auf `DONE` geht, wird aus der Warteschlange die nächste Runde
   eröffnet (ggf. automatisch gewürfelt, sobald voll). Dockt am `Finish`-Flow an.
5. **Punkte beim Abschluss**: das Finish-Modal (heute nur Gewinner-Team) um eine Eingabe der
   Ergebnisse erweitern — Modus **best of 1** oder **best of 3** (Activity-Eigenschaft?). Resultat
   in `sessions`/einer Score-Tabelle ablegen und über den `session.Notifier` (Teams) ausgeben.

## Akzeptanzkriterien
- Offene Sessions verschwinden nach Ablauf zuverlässig (`EXPIRED`).
- Re-Roll erzeugt eine neue, gültige 2v2-Verteilung.
- Eine Historie-Seite zeigt vergangene Sessions.

## Hinweise / Fallen
- Auto-Expire als periodischer Ticker; sauber beim Service-Stop beenden.
- Re-Roll nur in sinnvollen States erlauben (nach Würfeln, vor "DONE").
- Stats-Queries klein halten; kein Over-Engineering.
