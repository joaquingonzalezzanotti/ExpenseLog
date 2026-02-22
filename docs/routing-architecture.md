# Routing Architecture (`/`, `/app/*`, `/api/*`)

## Goal
Define and freeze the web routing contract so landing, app, and API can evolve independently.

## Canonical routes

1. `https://expenselog.com.ar/` -> public landing.
2. `https://expenselog.com.ar/app` and `https://expenselog.com.ar/app/*` -> authenticated app UI.
3. `https://expenselog.com.ar/api/*` -> backend API.

## Backend behavior in this repo

As of February 22, 2026, the Go server supports:

1. New namespaced routes:
   - `/app`, `/app/table`, `/app/settings`
   - `/app/*` static assets (for example `/app/style.css`, `/app/functions.js`)
   - `/api/*` for all auth/config/expenses/reconciliation/import-export/version endpoints
2. Legacy compatibility routes still active:
   - `/table`, `/settings`
   - root API routes (`/auth/*`, `/expense*`, etc.)
   - root static assets (`/style.css`, `/functions.js`, etc.)

This allows safe rollout without breaking existing links/sessions.

## Frontend contract

App templates now load:

1. assets from `/app/*`
2. auth provider entrypoint from `/api/auth/google`
3. API calls through `/api/*` (automatically prefixed in `internal/web/templates/functions.js`)

Landing remains on `/` and opens `/app`.

## Suggested Vercel rewrites (landing on Vercel, app/api on Railway)

`vercel.json`:

```json
{
  "rewrites": [
    {
      "source": "/app",
      "destination": "https://<railway-host>/app"
    },
    {
      "source": "/app/:path*",
      "destination": "https://<railway-host>/app/:path*"
    },
    {
      "source": "/api/:path*",
      "destination": "https://<railway-host>/api/:path*"
    }
  ]
}
```

Replace `<railway-host>` with your real Railway domain.

## Migration notes

1. Keep legacy routes enabled during transition.
2. Update external integrations to `/api/*` gradually.
3. After production stabilization, remove legacy routes in a separate hardening pass.
