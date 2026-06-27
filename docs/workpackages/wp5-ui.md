# Arbeitspaket 5 — UI/UX-Politur

> **Prompt für eine eigene Claude-Code-Session.** Lies zuerst `docs/PLAN.md` (Abschnitt 5a).
> Erwäge das `frontend-design`-Skill für die visuelle Richtung. Bestand nutzen: Pico CSS, Remixicon, htmx.

## Ziel
Aus der funktionalen UI (aus WP2) eine aufgeräumte, bewusst gestaltete kleine Web-App machen:
mobile-first, klare Zustände, konfigurierbares Branding, Light/Dark.

## Voraussetzungen
WP2 abgeschlossen (funktionale Lobby-UI existiert). Idealerweise auch WP1 (User im Header).

## Aufgaben
1. **Design-Richtung** festlegen (Typo, Akzentfarbe, Spacing) — bewusst, nicht Default-Template.
   Custom-CSS minimal in `assets/css/style.css`, via `make esbuild` bundeln.
2. **Lobby-Screen** politur für alle Zustände (siehe Plan 5a):
   - Kein-Session-CTA, offene Session (Teilnehmer/Avatare + Fortschritt "n / m"), gewürfelte Teams.
   - Saubere Empty-/Loading-/Error-States.
3. **Branding konfigurierbar**: App-Name/Titel + Akzentfarbe aus Config (keine hartcodierten
   firmenspezifischen Texte). `web/templates/base.html` entsprechend anpassen.
4. **Light/Dark**: Picos `data-theme` umschaltbar machen (statt fix `light`).
5. **Mobile-first** prüfen: Touch-Targets, Layout auf schmalen Screens, Lesbarkeit.
6. **Dezente Übergänge** beim htmx-Live-Update, damit das Poll-Refresh nicht "springt".

## Akzeptanzkriterien
- App wirkt auf Mobil und Desktop aufgeräumt und konsistent.
- Alle Lobby-Zustände inkl. Empty/Error sind gestaltet.
- Branding (Name/Farbe) ist per Config änderbar, keine Firmen-Texte im Code.
- Light/Dark funktioniert.

## Hinweise / Fallen
- Wenig Custom-CSS — Pico macht das meiste; nicht gegen das Framework kämpfen.
- Assets-Änderungen erfordern `make esbuild` + Rebuild (Assets sind embedded).
- Barrierearmut nicht vergessen (Kontraste, fokussierbare Buttons).
