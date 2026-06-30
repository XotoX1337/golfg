# Setting up Teams notifications

This guide sets up a **Microsoft Teams** channel that receives go LFG's posts —
"someone wants to play", "teams are set" and "match over" — each with a deep-link
back into the app. go LFG only ever **sends** to Teams (an outbound webhook);
there are no bots, no callbacks, and all interaction happens in the app itself.

> **You don't need this to try the app.** With `[teams]` left empty, go LFG runs
> fine — the posts are just written to the log instead of Teams (look for
> `teams post (log only, no webhook)`). The webhook is only needed if you want the
> announcements to actually show up in a channel.

Microsoft retired the old *"Incoming Webhook"* Office 365 connectors. The current
mechanism is a **Power Automate workflow** triggered by an HTTP request, which is
what this guide uses.

## What you'll end up with

One value that goes into your config (or an environment variable):

| Value | Where it comes from | Config key | Env var |
|-------|--------------------|------------|---------|
| Webhook URL | Power Automate workflow (below) | `teams.webhook_url` | `GOLFG_TEAMS_WEBHOOK_URL` |

The channel notification language is fixed by config (the channel has no
per-request locale): `teams.lang` / `GOLFG_TEAMS_LANG`, `"en"` (default) or
`"de"`.

You also want `app.base_url` set to the app's public URL so the deep-link buttons
in the cards point somewhere useful.

## 1. Create the "Kickern" channel

In the Teams team of your choice, create (or pick) a channel — e.g. **Kickern** —
where the announcements should land. Anyone who should see "let's play" pings
needs to be a member of that channel.

## 2. Create the workflow from the channel

The quickest path uses the built-in template:

1. In Teams, open the **Kickern** channel.
2. Click the **•••** (more options) next to the channel name → **Workflows**
   (or **Manage channel → Workflows**).
3. Search for and select the template
   **"Post to a channel when a webhook request is received"**.
4. Confirm the connection (sign in if prompted), pick the **Team** and the
   **Kickern** channel as the target, then **Add workflow / Create**.
5. Teams shows a **URL** — this is your webhook. **Copy it now** and keep it
   secret; anyone with the URL can post to the channel. This goes into
   `teams.webhook_url`.

That template's flow already takes the incoming request body and posts its
Adaptive Card attachment to the channel, which is exactly the shape go LFG sends
(see [Payload format](#payload-format) below) — no further editing required.

> Don't see "Workflows"? You can also start from
> [Power Automate](https://make.powerautomate.com) → **Create → Instant cloud
> flow** → trigger **"When a Teams webhook request is received"**, then add a
> **"Post card in a chat or channel"** action posting the trigger's
> `attachments`. See the manual setup note below.

## 3. Put the URL into go LFG's config

In `golfg.toml` (next to the binary):

```toml
[app]
base_url = "https://kicker.intranet"   # used for the deep-link button in cards

[teams]
webhook_url = ""                        # leave empty here; inject via ENV
lang = "en"                             # channel notification language: "en" or "de"
mention_players = true                  # @-mention drawn players in the "teams are set" post (set false to disable)
```

Then provide the secret URL via environment variable when starting the app:

```bash
export GOLFG_TEAMS_WEBHOOK_URL="<the workflow URL from step 2>"
./golfg
```

Everything can be supplied via ENV instead of the file, using the scheme
`GOLFG_<SECTION>_<KEY>`. ENV always wins over the file — handy for containers.
**Never commit the webhook URL** — it contains a secret token; only the
placeholder in `golfg.example.toml` belongs in the repo.

## 4. Test it

1. Start go LFG. Start a session ("I want to play") — within a moment a card
   should appear in the **Kickern** channel:
   *"⚽ Anton wants to play Tischfußball — 3 spot(s) left"* with a **Join the
   game** button.
2. Fill the session (4 players). A second card appears:
   *"It's on! Teams are set — Team A: … — Team B: …"*.
3. End the match in the app ("End match", pick a winner). A third card appears:
   *"🏁 Match over! — 🏆 Team A won Tischfußball — …"*.
4. Click a card button — it should open `app.base_url` and land you in the lobby.

If nothing appears, check the app log: a successful send logs
`teams post sent` with HTTP status `200`/`202`; failures log `teams: post failed`
or `teams: unexpected status`. Posts are best-effort and asynchronous, so a
broken webhook never blocks or crashes the app.

## Payload format

go LFG POSTs JSON the Power-Automate "Teams webhook" trigger understands: a
message whose single attachment is an Adaptive Card. Knowing the shape helps if
you build the flow manually or want to customize the card:

```json
{
  "type": "message",
  "attachments": [
    {
      "contentType": "application/vnd.microsoft.card.adaptive",
      "content": {
        "$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
        "type": "AdaptiveCard",
        "version": "1.4",
        "body": [
          { "type": "TextBlock", "text": "⚽ Anton wants to play Tischfußball", "size": "Large", "weight": "Bolder", "wrap": true },
          { "type": "TextBlock", "text": "3 spot(s) left — join in!", "wrap": true }
        ],
        "actions": [
          { "type": "Action.OpenUrl", "title": "Join the game", "url": "https://kicker.intranet/" }
        ]
      }
    }
  ]
}
```

A **manual** Power Automate flow should therefore: trigger on
**"When a Teams webhook request is received"**, then **"Post card in a chat or
channel"** using `triggerBody()?['attachments']` (the first attachment's
`content`) as the Adaptive Card.

## Notes & troubleshooting

- **Sparse on purpose:** go LFG posts on **session start**, **teams drawn** and
  **match over**. Joins and leaves are visible live in the app and are not posted,
  so the channel doesn't get noisy.
- **"… used a Workflow template to send this card":** Power Automate adds this
  attribution line (the workflow owner) automatically. It is **not** part of go
  LFG's payload and cannot be removed from it — it's a property of the Workflows
  posting mechanism. Removing it would mean posting via a different mechanism
  (e.g. a dedicated bot), which is out of scope here.
- **Notification language:** set `teams.lang` (`"en"`/`"de"`) to pick the channel
  language; it is independent of each user's in-app UI language.
- **@-mentioning players ("teams are set" post):** with `teams.mention_players`
  on (the default; `GOLFG_TEAMS_MENTION_PLAYERS`), the *"It's on! Teams are set"*
  post @-mentions the drawn players so they get a real Teams notification
  ("X mentioned you") instead of just seeing the channel post. Only the
  *teams-drawn* post mentions; *session started* and *match over* are unchanged.
  Two caveats, both expected Teams behavior, not bugs:
    - **SSO only:** a player is only mentionable if we know their Entra object id,
      which is captured at SSO login. Dev-login users (no SSO) and any legacy
      accounts without an OID render as a **plain name** — never a broken mention.
    - **Channel members only:** a mention renders as a clickable name for everyone,
      but only delivers a **notification** to players who are **members of the
      channel** the workflow posts to. Add the players to that channel for the
      pings to land. Set `mention_players = false` to turn pinging off entirely.
- **Custom "session started" headline:** set `branding.play_announcement`
  (or `GOLFG_BRANDING_PLAY_ANNOUNCEMENT`) to override the *"… wants to play …"*
  title line with your own wording. It is a small template with a single
  `{{.Name}}` placeholder for the creator's name, e.g.
  `play_announcement = "{{.Name}} wants to play!"` → *"Jane Doe wants to
  play!"*. The value is a **fixed literal** — it is not translated, so
  `teams.lang` no longer affects this line (it still drives the "n spots left"
  subtitle and the other cards). An empty value, or an invalid template, falls
  back to the localized default (which also includes the activity name). The
  matching in-app button label is `branding.play_cta`; see
  `golfg.example.toml`.
- **Outbound only / intranet hosting is fine:** the app just makes an outbound
  HTTPS POST to the workflow URL. Microsoft never calls back into the app, so the
  server does not need to be reachable from the internet.
- **Keep the URL secret:** it's a bearer credential. Rotate it by deleting and
  recreating the workflow if it leaks, then update `GOLFG_TEAMS_WEBHOOK_URL`.
- **Deep-links go to the lobby:** there is one active session at a time, so the
  card button links to `app.base_url`, which shows the current session.
