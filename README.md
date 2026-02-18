# ExpenseLog

ExpenseLog es una app web para llevar ingresos y gastos con foco en simplicidad y control.

## Usar la app
- Acceso web: https://www.expenselog.com.ar
- No requiere instalacion.
- Compatible con mobile y escritorio.

## Primeros pasos
1) Crea tu cuenta con email y confirma la verificacion. (O usa el acceso con tu cuenta de Google)
2) Inicia sesion.
3) Configura moneda base y categorias.
4) Registra gastos, ingresos y recurrentes.

## Funciones principales
- Multiusuario con datos aislados por cuenta.
- Multi-moneda ARS/USD/EUR (sin conversion automatica).
- Graficos y balances por moneda base.
- Etiquetas y categorias personalizadas.
- Importacion y exportacion CSV temporalmente deshabilitadas.

## Reglas de Recurrentes y Alertas de Liquidez
- Fuente de verdad:
  - `recurring_expenses` define la regla (ej: monto e intervalo).
  - `expenses` guarda instancias (incluye futuras).
- Balance ARS "actual" para alertas:
  - Se calcula con movimientos CA/legacy hasta hoy.
  - No descuenta gastos recurrentes futuros fuera de su ventana.
- Ventanas de alerta:
  - `7 a 5 dias`: notificacion informativa (`preview_7d`), aunque el saldo proyectado quede negativo.
  - `4 a 1 dias`: seguimiento (`monitor_4d`) o alerta critica (`risk_4d`) si el saldo proyectado queda menor a 0.
  - `hoy o vencido reciente`: solo informativa (`due`), nunca critica.
  - `reaparicion 24h`: si el usuario borra una notificacion, vuelve a mostrarse al entrar en ventana de 24h.
- API de alertas:
  - `/alerts/liquidity` devuelve `windowDays`, `criticalDays` y `reappearDays`.
  - Frontend consume esos valores para evitar numeros magicos duplicados.
- Orden y acumulacion:
  - Si hay multiples recurrentes cercanas, se proyectan en orden de vencimiento.
  - Puede pasar que la segunda recurrente sea la que vuelva negativo el saldo.

## Semantica de Fechas (Timezone)
- La app trata vencimientos recurrentes como eventos de calendario (no por hora exacta).
- Regla backend actual:
  - Se toma el dia calendario UTC (`Y-M-D`) del vencimiento para calcular `daysUntil`.
  - Esto evita parte de los corrimientos por zona horaria, pero documenta que un timestamp con offset extremo puede mapear al dia UTC anterior/siguiente.

## Cobertura de Tests de Liquidez
- Archivo: `internal/api/alerts_test.go`
- Casos cubiertos:
  - Fuera de ventana (sin alerta).
  - Preview 7d informativa con saldo >= 0 y < 0.
  - Ventana 4d con `monitor_4d` y `risk_4d`.
  - Vencimiento hoy y vencido reciente (informativo).
  - Acumulacion secuencial de recurrentes.
  - Bordes de timezone y dia calendario UTC.
- Ejecutar:
  - `go test ./internal/api -run Liquidity -v`
  - `go test ./...`
  - `node --test internal/web/templates/alerts_ui.test.js`

## Paleta UI oficial (Variante A)
- Light:
  - `--bg-primary: #F6F7F9`
  - `--bg-secondary: #FFFFFF`
  - `--text-primary: #1B2430`
  - `--text-secondary: #5A6778`
  - `--border: #D9E0E8`
  - `--accent: #2563EB`
  - `--danger: #DC2626`
- Dark:
  - `--bg-primary: #0B1220`
  - `--bg-secondary: #121C2E`
  - `--text-primary: #E6EDF6`
  - `--text-secondary: #9BAAC0`
  - `--border: #253247`
  - `--accent: #60A5FA`
  - `--danger: #EF4444`

## Cuenta y seguridad
- Sesiones seguras con opcion de recordar sesion.
- Recupero de contrasena por codigo.
- Login con Google cuando esta disponible.
