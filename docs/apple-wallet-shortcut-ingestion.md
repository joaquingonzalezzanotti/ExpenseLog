# Apple Wallet Shortcut ingestion (MVP)

## Proposito
Este flujo recibe eventos desde un Shortcut de Apple Wallet/Apple Pay, guarda el payload crudo para trazabilidad y, si hay datos suficientes, crea una transaccion borrador en ExpenseLog.

## Endpoints

### `POST /api/integrations/apple-wallet/debug`
- Requiere sesion de usuario (cookie de auth).
- Guarda body completo + headers utiles para debugging.
- Respuesta: `{ "status": "ok", "eventId": "..." }`.

### `GET /api/integrations/apple-wallet/token-status`
- Requiere sesion de usuario (cookie de auth).
- Devuelve si el usuario es premium y si ya tiene token activo.
- Respuesta ejemplo:
```json
{
  "premium": true,
  "has_token": true,
  "created_at": "2026-03-10T18:45:00Z",
  "last_used_at": "2026-03-10T19:10:00Z"
}
```

### `POST /api/integrations/apple-wallet/token`
- Requiere sesion de usuario (cookie de auth).
- Solo disponible para usuarios premium.
- Genera o rota un token por usuario (se guarda hasheado en DB).
- Respuesta ejemplo:
```json
{
  "token": "<TOKEN_PLAINTEXT>",
  "created_at": "2026-03-10T18:45:00Z"
}
```

### `POST /api/integrations/apple-wallet/ingest`
- Requiere `Authorization: Bearer <token>`.
- El token se valida contra `wallet_ingest_tokens`.
- Solo procesa eventos de usuarios premium.
- Guarda evento en `wallet_ingest_events`.
- Si llega `paymentMethod`, se usa para el metodo contable de la transaccion (`CA`, `TARJETA`, `EFECTIVO`).
- Si hay datos suficientes, crea transaccion borrador.
- Si no hay datos, marca evento como `needs_review`.

## Payload esperado
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

## Dedupe MVP
Se evita crear transaccion nueva cuando coincide:
- mismo `user_id`
- mismo `amount` redondeado a 2 decimales
- mismo merchant normalizado
- `paid_at` dentro de +/-10 minutos

## Limitaciones actuales
- El token se muestra en claro solo al generarse (si lo perdes, hay que rotarlo).
- Dedupe basico textual (sin fuzzy matching avanzado).
- `walletCategory` se guarda como sugerencia, no categoriza automaticamente.
- Las transacciones se crean como borrador implicito (`category = "Por revisar"`).
