# Migracion a Plan Pago en Railway (Sin Romper Produccion)

## Objetivo
Documentar como migrar de Free/Trial a plan pago en Railway y que hacer con el flujo temporal de "wake" (`/app/wake`) que se agrego para evitar el error 502 durante cold start.

Fecha de referencia: 2026-03-14.

## Resumen corto
1. Primero migra infraestructura (Railway) y valida estabilidad.
2. Despues, solo si queres, limpia el codigo temporal de wake.
3. No hagas ambas cosas en el mismo deploy.

## Fase 1: Infra (recomendada apenas pases a plan pago)

### 1) Cambios en Railway (entorno `production`)
1. Desactivar serverless/sleep para el servicio web principal (`ExpenseLog`).
2. Configurar replica minima en `1` (always-on).
3. Restart policy: `Always`.
4. Mantener networking igual:
   - Domain target port: `8080`.
   - App escuchando en `8080` (o `PORT` equivalente).
5. Deploy normal del branch estable (`main`).

### 2) Validacion post deploy (smoke test)
1. Abrir `/` sin sesion y verificar que carga landing.
2. Abrir `/` con sesion activa y verificar que redirige a `/app`.
3. Abrir `/app` directo y verificar que ya no aparece pantalla 502.
4. Revisar logs: no deberia haber ciclos frecuentes de `Starting Container` + `Stopping Container` por inactividad.

### 3) Criterio para pasar a Fase 2
Esperar 24-48 horas sin errores de disponibilidad en `production`.

## Fase 2: Limpieza del flujo temporal de wake (opcional)

El flujo temporal actual no rompe nada en plan pago. Podes dejarlo.
Si queres volver al flujo original, hacerlo en un PR/deploy separado:

1. PWA start URL:
   - Cambiar `start_url` de `/app/wake` a `/app` en `internal/web/templates/manifest.json`.
2. Service worker:
   - Quitar `/app/wake` de `APP_ROUTES` en `internal/web/templates/sw.js`.
   - Volver a incrementar `SW_VERSION` para forzar recache.
3. Ruta temporal:
   - Quitar handler `/app/wake` en `cmd/expenselog/main.go`.
   - Eliminar template `internal/web/templates/wake.html`.
4. Landing auto-redirect:
   - Decidir si mantener o quitar la deteccion de sesion en `internal/web/templates/landing.html`.
   - Recomendacion: mantenerla (mejora UX y no depende de plan).

## Nota sobre PWA en iOS/Android
Si cambias `start_url` de nuevo a `/app`, usuarios con acceso directo viejo pueden quedar cacheados.
Recomendado avisar:
1. cerrar app instalada,
2. reabrir una vez desde navegador,
3. si persiste, eliminar acceso directo y volver a agregarlo.

## Rollback rapido
Si algo falla tras la migracion:
1. Revertir ultimo deploy de codigo.
2. Mantener replica `1` y restart `Always`.
3. Verificar de nuevo target port `8080` y health endpoint.

## Checklist final
- [ ] Plan pago activo y serverless desactivado en `production`.
- [ ] Replica minima en `1`.
- [ ] Restart policy en `Always`.
- [ ] Smoke tests de `/`, `/app`, login y PWA OK.
- [ ] (Opcional) Limpieza de `/app/wake` en deploy separado.
