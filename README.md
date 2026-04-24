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