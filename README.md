# ExpenseLog 💸

ExpenseLog es una app web para registrar ingresos/gastos, manejar recurrentes y controlar liquidez por moneda base.

## 🌐 App

- Produccion: `https://www.expenselog.com.ar`

## ✨ Funcionalidades clave

- Autenticacion con email/password y Google.
- Movimientos: gasto, ingreso y reintegro.
- Recurrentes con generacion de instancias.
- Alertas de liquidez (preview 7d + riesgo 4d).
- Dashboard, tabla y configuracion en UI responsive.

## 🧱 Stack

- Backend: Go (`net/http`)
- Storage: PostgreSQL
- Frontend: HTML/CSS/JS embebido (`go:embed`)

## 🚀 Ejecutar local

1. Configura variables de entorno:

```bash
STORAGE_TYPE=postgres
STORAGE_URL=localhost:5432/expenselog?sslmode=disable
STORAGE_USER=postgres
STORAGE_PASS=postgres
```

Opcionales:

```bash
APP_BASE_URL=http://localhost:8080
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REDIRECT_URL=http://localhost:8080/auth/google/callback
SMTP_HOST=...
SMTP_PORT=587
SMTP_USER=...
SMTP_PASS=...
SMTP_FROM=...
```

2. Inicia la app:

```bash
go run ./cmd/expenselog -port 8080
```

3. Abre:

```text
http://localhost:8080
```

## 🧪 Tests

```bash
go test ./...
node --test internal/web/templates/alerts_ui.test.js
```

## 🐳 Docker

```bash
docker build -t expenselog .
docker run --rm -p 8080:8080 expenselog
```

## 📁 Estructura

- `cmd/expenselog`: entrypoint de la app
- `internal/api`: handlers y reglas de negocio
- `internal/storage`: acceso a datos
- `internal/web/templates`: UI y assets embebidos
