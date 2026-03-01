# ExpenseLog AI Parser

Microservicio headless para extraer movimientos financieros desde:
- texto libre (chat/OCR),
- imagenes de comprobantes,
- PDF de comprobantes.

Este servicio esta pensado como fallback/inteligencia para `bot-prod`.

## Arquitectura recomendada

`Telegram Bot -> AI Parser -> ExpenseLog API`

Flujo sugerido:
1. Si llega texto: bot llama directo a `/api/parse`.
2. Si llega imagen/PDF: bot intenta su parser interno.
3. Si falla o confianza baja: bot llama `/api/parse` con `media`.
4. Bot muestra resumen al usuario.
5. Bot confirma y recien ahi guarda en ExpenseLog (`/expense`).

## Requisitos

- Node.js 20+
- Gemini API Key

## Instalacion local

1. `npm install`
2. Copiar `.env.example` a `.env` y completar variables.
3. `npm run dev`

## Endpoints

### `GET /healthz`
Estado del servicio.

### `POST /api/parse`
Extrae una transaccion estructurada.

Headers:
- `Content-Type: application/json`
- `X-Internal-Token: <token>` (solo si definis `INTERNAL_TOKEN`)

Body (texto):
```json
{
  "text": "Gaste 1600 en super hoy",
  "context_date": "2026-03-01T11:30:00-03:00"
}
```

Body (imagen o PDF en base64):
```json
{
  "context_date": "2026-03-01T11:30:00-03:00",
  "media": {
    "mime_type": "image/jpeg",
    "data_base64": "<BASE64_SIN_DATA_URL>",
    "filename": "comprobante.jpg"
  }
}
```

Body (hibrido: texto + media):
```json
{
  "text": "Creo que fue ayer a la tarde",
  "media": {
    "mime_type": "application/pdf",
    "data_base64": "<BASE64_DEL_PDF>"
  }
}
```

Respuesta:
```json
{
  "type": "expense",
  "amount": 1600,
  "currency": "ARS",
  "datetime_iso": "2026-03-01T13:10:00-03:00",
  "counterparty": "Super Salto",
  "reference": "",
  "motive": "",
  "source_app": "BANK",
  "confidence": {
    "type": 0.95,
    "amount": 0.92,
    "datetime_iso": 0.84,
    "counterparty": 0.77,
    "overall": 0.87
  },
  "missing_required": [],
  "warnings": []
}
```

## Variables de entorno

- `GEMINI_API_KEY` (requerida)
- `GEMINI_MODEL` (default: `gemini-2.5-flash`)
- `PORT` (default: `3000`)
- `LOG_LEVEL` (default: `info`)
- `INTERNAL_TOKEN` (opcional, recomendado en prod)
- `MAX_MEDIA_BYTES` (default: `8388608`)
- `JSON_BODY_LIMIT` (default: `20mb`)

## Railway (prod)

Desplegar como servicio aparte (`ai-parser-prod`) dentro del mismo proyecto donde ya tenes `bot-prod` y `ExpenseLog`.
