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

## Quickstart local

### 1) Requisitos

- Go 1.25+
- PostgreSQL 14+ (o container Docker)
- Node 20+ (solo para tests de UI)
- Python 3 (script de validacion UTF-8)

### 2) Levantar PostgreSQL rapido (Docker)

```bash
docker run --name expenselog-pg \
  -e POSTGRES_USER=expenselog \
  -e POSTGRES_PASSWORD=expenselog \
  -e POSTGRES_DB=expenselog \
  -p 5432:5432 \
  -d postgres:16
```

### 3) Configurar variables de entorno minimas

PowerShell:

```powershell
$env:STORAGE_TYPE="postgres"
$env:STORAGE_URL="localhost:5432/expenselog"
$env:STORAGE_USER="expenselog"
$env:STORAGE_PASS="expenselog"
$env:STORAGE_SSL="disable"
$env:PORT="8080"
```

Bash:

```bash
export STORAGE_TYPE=postgres
export STORAGE_URL=localhost:5432/expenselog
export STORAGE_USER=expenselog
export STORAGE_PASS=expenselog
export STORAGE_SSL=disable
export PORT=8080
```

### 4) Ejecutar la app

```bash
go run ./cmd/expenselog
```

Abrir: `http://localhost:8080`

La app crea tablas e indices automaticamente al iniciar.

## Variables de entorno

### Core (obligatorias)

- `STORAGE_TYPE=postgres`
- `STORAGE_URL` (formato: `host:puerto/database`)
- `STORAGE_USER`
- `STORAGE_PASS`
- `STORAGE_SSL` (`disable`, `require`, `verify-full`, `verify-ca`)

### Core (recomendadas)

- `PORT` (default: `8080`)
- `TRUST_PROXY_HEADERS=true` (si estas detras de reverse proxy)
- `CONTENT_SECURITY_POLICY` (si queres override de CSP default)

### Bootstrap inicial (opcional)

Si definis estas variables, se crea automaticamente un usuario inicial:

- `BOOTSTRAP_EMAIL`
- `BOOTSTRAP_PASSWORD`

### Email (opcional)

SMTP:

- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USER`
- `SMTP_PASS`
- `SMTP_FROM`
- `SMTP_FROM_NAME`

Brevo API:

- `BREVO_API_KEY`

Branding de mails:

- `APP_NAME`
- `APP_BASE_URL`
- `APP_LOGO_URL`
- `SUPPORT_EMAIL`
- `EMAIL_PRIMARY_COLOR`
- `EMAIL_ACCENT_COLOR`

### Auth Google (opcional)

- `GOOGLE_CLIENT_ID`
- `GOOGLE_CLIENT_SECRET`
- `GOOGLE_REDIRECT_URL`

### Integraciones bots (opcionales)

Telegram:

- `EXPENSELOG_BOT_INTERNAL_SECRET`
- `TELEGRAM_BOT_USERNAME`

WhatsApp (Kapso):

- `KAPSO_WEBHOOK_SECRET`
- `KAPSO_API_KEY`
- `WHATSAPP_KAPSO_NUMBER`
- `WHATSAPP_KAPSO_DEFAULT_MESSAGE`
- `KAPSO_MEDIA_ALLOWED_HOSTS`

AI parser para WhatsApp:

- `AI_PARSER_BASE_URL`
- `AI_PARSER_PARSE_PATH`
- `AI_PARSER_INTERNAL_TOKEN`
- `AI_PARSER_API_KEY`
- `AI_PARSER_API_KEY_HEADER`
- `AI_PARSER_TIMEOUT_MS`
- `WHATSAPP_AI_FALLBACK_ENABLED`
- `WHATSAPP_OCR_FALLBACK_ENABLED`
- `WHATSAPP_OCR_LANGS` (o `OCR_LANGS`)

## Endpoints operativos

- Health: `GET /health` y `GET /api/health`
- Ready: `GET /ready` y `GET /api/ready`
- Metrics: `GET /metrics` y `GET /api/metrics`
- Version: `GET /version` y `GET /api/version`

## Calidad y tests

### Suite recomendada antes de merge/deploy

```bash
python3 scripts/check_utf8.py
go test ./...
node internal/web/templates/alerts_ui.test.js
node internal/web/templates/cashflow_ui.test.js
node internal/web/templates/onboarding_ui.test.js
```

CI actual en `.github/workflows/ci.yml`.

## Deploy

### Docker

Build:

```bash
docker build -t expenselog:local .
```

Run:

```bash
docker run --rm -p 8080:8080 \
  -e STORAGE_TYPE=postgres \
  -e STORAGE_URL=host.docker.internal:5432/expenselog \
  -e STORAGE_USER=expenselog \
  -e STORAGE_PASS=expenselog \
  -e STORAGE_SSL=disable \
  expenselog:local
```

### Kubernetes

Base manifests en `kubernetes/`.

Orden sugerido:

```bash
kubectl apply -f kubernetes/namespace.yml
kubectl apply -f kubernetes/expenselog-configmap.yml
kubectl apply -f kubernetes/expenselog-pvc.yml
kubectl apply -f kubernetes/expenselog-deployment.yml
kubectl apply -f kubernetes/expenselog-svc.yml
kubectl apply -f kubernetes/expenselog-ingress.yml
```

## Seguridad y datos sensibles

- No subir secretos a git (`.env`, tokens, passwords).
- El seed de screenshots `scripts/seed_screenshot_demo.sql` esta ignorado por `.gitignore`.
- No usar credenciales demo en produccion.
- Revisar CSP/HSTS en entorno con TLS.

## Checklist pre-produccion

- [ ] `go test ./...` y tests UI en verde
- [ ] Variables productivas cargadas en entorno
- [ ] SMTP funcionando (flujo reset/verify)
- [ ] Backups de PostgreSQL activos
- [ ] `/health` y `/ready` monitoreados
- [ ] Smoke test manual: registro, login, alta gasto, editar, eliminar, recurrente, conciliacion, export
- [ ] Validar vista mobile y desktop en `/app`, `/app/table`, `/app/analisis`, `/app/settings`
- [ ] Revisar que no haya datos mock en DB productiva

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

