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

## Akzeptanzkriterien
- Offene Sessions verschwinden nach Ablauf zuverlässig (`EXPIRED`).
- Re-Roll erzeugt eine neue, gültige 2v2-Verteilung.
- Eine Historie-Seite zeigt vergangene Sessions.

## Hinweise / Fallen
- Auto-Expire als periodischer Ticker; sauber beim Service-Stop beenden.
- Re-Roll nur in sinnvollen States erlauben (nach Würfeln, vor "DONE").
- Stats-Queries klein halten; kein Over-Engineering.
