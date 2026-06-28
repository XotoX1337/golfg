# Setting up Microsoft Entra SSO

This guide walks you through registering **go LFG** as an application in Microsoft
Entra ID (formerly Azure AD) so colleagues can sign in with their Microsoft work
account. No prior Azure knowledge is required — just follow the steps in order.

> **You don't need this to try the app.** With `[auth]` left empty, go LFG runs in
> **dev mode**: a local login where you just type a name. SSO is only needed for a
> real deployment inside your organization.

## What you'll end up with

Four values that go into your config (or environment variables):

| Value | Where it comes from | Config key | Env var |
|-------|--------------------|------------|---------|
| Tenant ID | Entra → app overview | `auth.tenant_id` | `GOLFG_AUTH_TENANT_ID` |
| Client ID | Entra → app overview | `auth.client_id` | `GOLFG_AUTH_CLIENT_ID` |
| Client secret | Entra → Certificates & secrets | `auth.client_secret` | `GOLFG_AUTH_CLIENT_SECRET` |
| Public URL | Where the app is reachable | `app.base_url` | `GOLFG_APP_BASE_URL` |

You need permission to register applications in your tenant. If "App
registrations" is greyed out, ask a Global Administrator or someone with the
*Application Developer* role to do the registration (or grant you the role).

## 1. Register the application

1. Open the [Microsoft Entra admin center](https://entra.microsoft.com) (or the
   Azure portal → **Microsoft Entra ID**).
2. Go to **Identity → Applications → App registrations** and click
   **+ New registration**.
3. Fill in:
   - **Name**: e.g. `go LFG` (only shown to your users on the consent screen).
   - **Supported account types**: choose
     **Accounts in this organizational directory only (single tenant)**.
   - **Redirect URI**: select platform **Web** and enter your callback URL —
     this is your app's public URL plus `/auth/callback`, for example:
     ```
     https://kicker.intranet/auth/callback
     ```
     ⚠️ This must match `app.base_url` + `/auth/callback` **exactly** (scheme,
     host, path, no trailing slash). Entra rejects anything that doesn't match
     character for character. For local testing you may add a second redirect URI
     such as `http://localhost:9000/auth/callback`.
4. Click **Register**.

## 2. Copy the Tenant ID and Client ID

On the app's **Overview** page, copy:

- **Application (client) ID** → `auth.client_id`
- **Directory (tenant) ID** → `auth.tenant_id`

Use the tenant **ID** (a GUID), not a domain name — go LFG validates tokens
against the issuer `https://login.microsoftonline.com/<tenant_id>/v2.0`.

## 3. Create a client secret

1. Go to **Certificates & secrets → Client secrets → + New client secret**.
2. Add a description (e.g. `golfg`) and pick an expiry (e.g. 12 or 24 months).
   Note the date — you'll have to rotate the secret before it expires.
3. Click **Add**, then **immediately copy the secret _Value_** (not the
   *Secret ID*). It is shown only once.

This value goes into `auth.client_secret`. **Never commit it** — prefer injecting
it via the `GOLFG_AUTH_CLIENT_SECRET` environment variable on the server.

## 4. Confirm the API permissions / scopes

go LFG requests the standard OpenID Connect scopes `openid`, `profile` and
`email`. These map to the delegated **Microsoft Graph** permissions
`openid`, `profile`, `email`, which are usually present by default under
**API permissions**. If they're missing:

1. **API permissions → + Add a permission → Microsoft Graph →
   Delegated permissions**.
2. Add `openid`, `profile`, `email` and click **Add permissions**.

These are low-privilege, user-consentable permissions (just sign-in and basic
profile), so admin consent is normally not required. The app reads the user's
object id (`oid`), name and email from the ID token only — it does not call Graph.

## 5. Put the values into go LFG's config

In `golfg.toml` (next to the binary):

```toml
[app]
base_url = "https://kicker.intranet"   # must match the redirect URI host/scheme

[auth]
tenant_id     = "00000000-0000-0000-0000-000000000000"
client_id     = "11111111-1111-1111-1111-111111111111"
client_secret = ""                      # leave empty here; inject via ENV

[session]
cookie_secure = true                    # required when served over HTTPS
```

Then provide the secret via environment variable when starting the app:

```bash
export GOLFG_AUTH_CLIENT_SECRET="<the secret value from step 3>"
./golfg
```

Everything can be supplied via ENV instead of the file, using the scheme
`GOLFG_<SECTION>_<KEY>` (e.g. `GOLFG_AUTH_TENANT_ID`). ENV always wins over the
file — handy for containers.

## 6. Test the login

1. Start go LFG. The log should say `auth: Entra SSO enabled` (if it says
   `DEV mode`, one of the three `[auth]` values is still empty).
2. Open `app.base_url` in a browser. You should be redirected to the Microsoft
   sign-in page.
3. Sign in. On success you're sent back to the lobby and your name appears in the
   header. Your account is cached in the local database (object id, name, email).

## Notes & troubleshooting

- **Redirect URI mismatch (AADSTS50011):** the URL in Entra doesn't exactly match
  `app.base_url` + `/auth/callback`. Check scheme (`https` vs `http`), host, and
  trailing slashes.
- **Cookie not kept / login loops:** if you serve over HTTPS, set
  `session.cookie_secure = true`. For plain-HTTP local dev, keep it `false`
  (a `Secure` cookie is dropped over http).
- **Intranet hosting is fine:** SSO works through the user's **browser redirect**
  to Microsoft and back. The server never needs to be reachable *from* the
  internet — only the user's browser does. No inbound callbacks from Microsoft
  are required.
- **Secret expired:** create a new client secret (step 3) and update
  `GOLFG_AUTH_CLIENT_SECRET`. Old secrets can be deleted afterwards.
