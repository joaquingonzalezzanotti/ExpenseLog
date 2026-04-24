<p align="center">
  <img src="/assets/ExpenseLogLogoText.png" alt="ExpenseLog" width="220" />
</p>

<h1 align="center">ExpenseLog</h1>

<p align="center">
  Control de ingresos, gastos y liquidez mensual con foco en uso real para Argentina.
</p>

<p align="center">
  <a href="https://www.expenselog.com.ar"><b>Web en produccion</b></a>
</p>

## Que es ExpenseLog

ExpenseLog es una web app para seguimiento financiero personal:

- Registro rapido de movimientos (gasto, ingreso, reintegro).
- Dashboard mensual con KPIs y alertas.
- Vista tabla y calendario para analisis diario.
- Gestion de categorias y gastos recurrentes.
- Conciliacion de saldo para alinear saldo app vs saldo real.
- Exportes mensuales (XLSX y PDF).
- Integraciones de bots (Telegram y WhatsApp).

## Estado del proyecto

- Branch estable de publicacion: `main`
- Branch de desarrollo continuo: `dev`
- Backend en Go (`net/http`) con PostgreSQL
- Frontend server-rendered + JS vanilla

## Capturas

Capturas disponibles en [`docs/screenshots/`](docs/screenshots/):

- Dashboard: `dashboard-desktop-light.png`
- Tabla: `table-desktop-light.png`
- Settings: `settings-desktop-light.png`
- Calendario: `calendar-desktop-light.png`

Guia de naming y recomendaciones en [`docs/screenshots/README.md`](docs/screenshots/README.md).

## Rutas principales

- Landing publica: `/`
- App principal: `/app`
- Tabla: `/app/table`
- Analisis: `/app/analisis`
- Configuracion: `/app/settings`
- API versionada: `/api/*`

Tambien se mantienen rutas legacy (`/table`, `/settings`, etc.) para compatibilidad.

## Stack tecnico

- Go `1.25.x`
- PostgreSQL
- HTML + CSS + JS (vanilla)
- Chart.js (visualizacion)
- Docker (imagen de produccion)
- Manifiestos Kubernetes en `kubernetes/`


## Kit de lanzamiento para Reddit (r/devsarg)

### Estructura recomendada del post

1. Problema real: "me costaba ver en que se me iba la plata mes a mes".
2. Solucion: ExpenseLog (dashboard, tabla, calendario, conciliacion, exportes).
3. Stack: Go + Postgres + frontend vanilla.
4. Aprendizajes tecnicos (1-2 puntos concretos).
5. Link a demo/web y pedido de feedback puntual.

### Borrador corto (editable)

> Estoy lanzando ExpenseLog, una app web para controlar gastos e ingresos con foco en uso real (dashboard mensual, tabla, calendario, conciliacion de saldo y Reportes).  
> La hice con Go + PostgreSQL + frontend vanilla.  
> Me serviria feedback tecnico sobre UX mobile, performance y prioridades de features.  
> Acceso: https://www.expenselog.com.ar

### Assets para el post

- 1 captura dashboard
- 1 captura tabla/calendario
- 1 gif corto de flujo (opcional)
- 3 bullets de diferenciales concretos

## Estructura del repo

```text
cmd/expenselog/                 # entrypoint y server HTTP
internal/api/                   # handlers y logica API
internal/storage/               # capa Postgres
internal/web/templates/         # HTML/CSS/JS de la app
docs/                           # documentacion funcional y tecnica
kubernetes/                     # manifiestos K8s
expenselog-ai-parser/           # microservicio parser AI (opcional)
telegram-receipt-bot/           # bot Telegram (opcional)
```

## Licencia

Copyright (c) 2026 Joaquin Gonzalez Zanotti.

