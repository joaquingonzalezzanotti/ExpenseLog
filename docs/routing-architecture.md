# Arquitectura de Ruteo (`/`, `/app/*`, `/api/*`)

## Objetivo
Definir y congelar el contrato de ruteo web para que landing, app y API puedan evolucionar de forma independiente.

## Rutas canónicas

1. `https://expenselog.com.ar/` -> landing pública.
2. `https://expenselog.com.ar/app` y `https://expenselog.com.ar/app/*` -> interfaz de la app autenticada.
3. `https://expenselog.com.ar/api/*` -> API backend.

## Comportamiento del backend en este repositorio

Al 22 de febrero de 2026, el servidor Go soporta:

1. Nuevas rutas con espacio de nombres:
  - `/app`, `/app/table`, `/app/settings`
  - activos estáticos en `/app/*` (por ejemplo `/app/style.css`, `/app/functions.js`)
  - `/api/*` para todos los endpoints de auth/config/expenses/reconciliation/import-export/version
2. Rutas de compatibilidad heredadas aún activas:
  - `/table`, `/settings`
  - rutas raíz de API (`/auth/*`, `/expense*`, etc.)
  - activos estáticos en la raíz (`/style.css`, `/functions.js`, etc.)

Esto permite un despliegue seguro sin romper enlaces o sesiones existentes.

## Contrato del frontend

Las plantillas de la app ahora cargan:

1. activos desde `/app/*`
2. punto de entrada del proveedor de autenticación desde `/api/auth/google`
3. llamadas a la API a través de `/api/*` (prefijadas automáticamente en `internal/web/templates/functions.js`)

El landing permanece en `/` y abre `/app`.

## Reescrituras sugeridas para Vercel (landing en Vercel, app/api en Railway)

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

Reemplaza `<railway-host>` por tu dominio real de Railway.

## Notas de migración

1. Mantener las rutas heredadas habilitadas durante la transición.
2. Actualizar gradualmente las integraciones externas a `/api/*`.
3. Tras la estabilización en producción, eliminar las rutas heredadas en una pasada de endurecimiento separada.
4. Nota: se puede usar un commit de documentación sin cambios para forzar la detección de auto-despliegue en CI/CD cuando el seguimiento de ramas se retrasa.
