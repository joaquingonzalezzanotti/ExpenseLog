# ExpenseLog 💸

ExpenseLog es una app web para registrar ingresos/gastos, manejar recurrentes y controlar liquidez por moneda base.

## 🌐 App

- Produccion: `https://www.expenselog.com.ar`
- Uso recomendado: acceder directamente desde la web (no requiere instalacion local).

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

## 📁 Estructura

- `cmd/expenselog`: entrypoint de la app
- `internal/api`: handlers y reglas de negocio
- `internal/storage`: acceso a datos
- `internal/web/templates`: UI y assets embebidos
