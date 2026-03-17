# Apple Wallet Shortcut ingestion (MVP)

## Proposito
Este flujo recibe eventos desde un Shortcut de Apple Wallet/Apple Pay, guarda el payload crudo para trazabilidad y, si hay datos suficientes, crea una transaccion borrador en ExpenseLog.

## Flujo E2E
1. Usuario premium autenticado genera (o rota) un token de ingest.
2. El Shortcut envia un `POST /api/integrations/apple-wallet/ingest` con `Authorization: Bearer <token>`.
3. Backend valida token + plan premium, persiste el evento y calcula `status/confidence`.
4. Si el payload alcanza, crea borrador en `expenses`; si no, deja el evento en `needs_review`.
5. Si detecta duplicado, no crea transaccion nueva y marca el evento como `duplicate`.

## Endpoints

### `POST /api/integrations/apple-wallet/debug`
- Auth: requiere sesion de usuario (`expense_session`).
- Metodo permitido: `POST`.
- Guarda payload JSON completo + headers relevantes (`content-type`, `user-agent`, `x-forwarded-for`, `x-forwarded-proto`).
- Persiste evento con:
  - `source = apple_wallet_shortcut`
  - `status = received`
  - `confidence = low`
- Respuesta OK: `201 Created`.
```json
{
  "status": "ok",
  "eventId": "..."
}
```

### `GET /api/integrations/apple-wallet/token-status`
- Auth: requiere sesion de usuario.
- Metodo permitido: `GET`.
- Devuelve estado premium + existencia de token activo del usuario.
- Comportamiento:
  - si no es premium: `{"premium":false,"has_token":false}`
  - si es premium sin token: `{"premium":true,"has_token":false}`
  - si es premium con token: incluye `created_at` y opcional `last_used_at`
- Respuesta OK: `200 OK`.

### `POST /api/integrations/apple-wallet/token`
- Auth: requiere sesion de usuario.
- Metodo permitido: `POST`.
- Solo premium; caso contrario devuelve `403` con `code = premium_required`.
- Genera token aleatorio de 24 bytes (`48` hex chars).
- Persiste solo hash SHA-256 en `wallet_ingest_tokens` (no guarda token plano).
- Hace upsert por `user_id` (rota token existente y limpia `last_used_at`).
- Respuesta OK: `200 OK`.
```json
{
  "token": "<TOKEN_PLAINTEXT>",
  "created_at": "2026-03-10T18:45:00Z"
}
```

### `POST /api/integrations/apple-wallet/ingest`
- Auth: `Authorization: Bearer <token>`.
- Metodo permitido: `POST`.
- No usa cookie de sesion.
- Valida token por hash contra `wallet_ingest_tokens`.
- Revalida plan premium del owner del token.
- Intenta actualizar `last_used_at` del token.

#### Reglas de normalizacion
- `amount`: redondea a 2 decimales (`NaN/Inf => 0`).
- `merchant`: usa `merchant`; si viene vacio, usa `merchantRaw`.
- Merchant se sanitiza y normaliza (lowercase + trim + espacios colapsados).
- `paymentMethod`: solo acepta `CA`, `TARJETA`, `EFECTIVO`; fallback `CA`.
- `source`: si viene vacio, usa `apple_wallet_shortcut`.
- `paidAt`: si falta pero hay datos suficientes, usa `time.Now().UTC()`.

#### Criterio de suficiencia
- Se crea borrador cuando:
  - `amount > 0`, y
  - hay `merchant` normalizado, o (`cardLabel` no vacio y `paidAt` presente).
- Si no cumple, el evento queda en `needs_review` y devuelve `202 Accepted`.

#### Confidence actual
- `high`: `amount > 0` + merchant + `paidAt`.
- `medium`: `amount > 0` + (merchant o (`paidAt` + `cardLabel`)).
- `low`: resto.

#### Dedupe MVP
No crea transaccion nueva si existe evento del mismo usuario con:
- mismo `amount` (round 2 decimales),
- mismo merchant normalizado (match exacto),
- `paid_at` dentro de `+/-10 minutos`,
- estado previo en `draft_transaction_created | duplicate | needs_review | received`.

#### Mapeo al borrador en `expenses`
- `name`: `merchant` -> `merchantRaw` -> `"Apple Wallet purchase"`.
- `flow`: fijo `"expense"` (se guarda como monto negativo).
- `category`: `"Por revisar"`.
- `source`: `paymentMethod` normalizado (`CA/TARJETA/EFECTIVO`).
- `card`: `cardLabel`.
- `currency`: moneda del usuario (`GetCurrency`).
- `date`: `paidAt`.
- `systemOrigin`: `apple_wallet_shortcut_draft`.

#### Respuestas principales
- `201 Created`: transaccion creada (`status = draft_transaction_created`).
- `200 OK`: duplicado (`status = duplicate`).
- `202 Accepted`: requiere revision (`status = needs_review`).
- `401 Unauthorized`: token ausente/invalido.
- `403 Forbidden`: usuario no premium.
- `400 Bad Request`: body invalido o monto invalido.

## Payload recomendado
Todos los campos son opcionales excepto que para crear borrador se requiere cumplir el criterio de suficiencia.

```json
{
  "amount": 12500,
  "merchant": "Starbucks",
  "merchantRaw": "STARBUCKS STORE 2143",
  "cardLabel": "Visa Galicia",
  "walletCategory": "Food & Drink",
  "paymentMethod": "TARJETA",
  "paidAt": "2026-03-10T14:32:00-03:00",
  "source": "apple_wallet_shortcut",
  "rawPayload": {
    "shortcutInput": {
      "amount": 12500,
      "merchant": "Starbucks"
    }
  }
}
```

## cURL local

### Ver estado de token
```bash
curl -X GET http://localhost:8080/api/integrations/apple-wallet/token-status \
  -H 'Cookie: expense_session=<SESSION_COOKIE>'
```

### Obtener/rotar token
```bash
curl -X POST http://localhost:8080/api/integrations/apple-wallet/token \
  -H 'Cookie: expense_session=<SESSION_COOKIE>'
```

### Ingest
```bash
curl -X POST http://localhost:8080/api/integrations/apple-wallet/ingest \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <SHORTCUT_TOKEN>' \
  -d '{
    "amount": 12500,
    "merchant": "Starbucks",
    "merchantRaw": "STARBUCKS STORE 2143",
    "cardLabel": "Visa Galicia",
    "walletCategory": "Food & Drink",
    "paymentMethod": "TARJETA",
    "paidAt": "2026-03-10T14:32:00-03:00",
    "source": "apple_wallet_shortcut",
    "rawPayload": {"shortcutInput":{"amount":12500,"merchant":"Starbucks"}}
  }'
```

## Persistencia
- Tabla de tokens: `wallet_ingest_tokens`.
- Tabla de eventos: `wallet_ingest_events`.
- El token plano solo se muestra al crearlo/rotarlo.

## Limitaciones actuales
- Dedupe textual exacto (sin fuzzy matching).
- `walletCategory` se guarda como metadata, no auto-categoriza.
- El borrador siempre sale con categoria `"Por revisar"`.
