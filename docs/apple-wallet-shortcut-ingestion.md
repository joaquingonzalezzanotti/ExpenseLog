# Apple Wallet Shortcut ingestion (MVP)

## Propósito
Este MVP recibe eventos enviados por un Shortcut de Apple Wallet/Apple Pay, guarda el payload crudo para trazabilidad y, si hay datos suficientes, crea una transacción borrador en ExpenseLog.

## Endpoints

### `POST /api/integrations/apple-wallet/debug`
- Requiere sesión de usuario (cookie de auth normal).
- Guarda el body completo + headers útiles para debugging.
- Respuesta: `{ "status": "ok", "eventId": "..." }`.

### `POST /api/integrations/apple-wallet/ingest`
- Requiere `Authorization: Bearer <token>`.
- Token configurable por variable de entorno:
  - `EXPENSELOG_SHORTCUT_INGEST_TOKENS=user_id_1:token_1,user_id_2:token_2`
- Guarda evento en `wallet_ingest_events`.
- Si hay datos suficientes, crea transacción borrador.
- Si no hay datos, guarda evento y lo marca `needs_review`.

## Payload esperado
```json
{
  "amount": 12500,
  "merchant": "Starbucks",
  "merchantRaw": "STARBUCKS STORE 2143",
  "cardLabel": "Visa Galicia",
  "walletCategory": "Food & Drink",
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

### Debug
```bash
curl -X POST http://localhost:8080/api/integrations/apple-wallet/debug \
  -H 'Content-Type: application/json' \
  -H 'Cookie: expense_session=<SESSION_COOKIE>' \
  -d '{"sample":true,"note":"debug"}'
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
    "paidAt": "2026-03-10T14:32:00-03:00",
    "source": "apple_wallet_shortcut",
    "rawPayload": {"shortcutInput":{"amount":12500,"merchant":"Starbucks"}}
  }'
```

## Dedupe MVP
Se evita crear transacción nueva cuando coincide:
- mismo `user_id`
- mismo `amount`
- mismo merchant normalizado
- `paid_at` dentro de +/-10 minutos

## Limitaciones actuales
- Auth por token estático en env (sin rotación/expiración).
- Dedupe básico textual (no fuzzy matching avanzado).
- `walletCategory` solo se guarda como sugerencia, no categoriza automáticamente.
- Las transacciones se crean como borrador implícito (`category = "Por revisar"`).

## Futuro (App Intents / acción nativa)
- Firma de requests por dispositivo.
- Gestión de tokens por usuario desde UI.
- Matching inteligente de comercios y categorías.
- Flujo de revisión/aprobación explícito de borradores.
