# Telegram Receipt Bot

Este bot se ejecuta desde `dist/` (codigo compilado).

## Requisitos

- Node.js 18.18+ (recomendado 20 LTS)
- PostgreSQL accesible por `DATABASE_URL`
- Binarios del sistema:
  - `tesseract` (OCR)
  - `pdftoppm` (Poppler, para render de PDF cuando no se extrae texto)

## Setup en una PC nueva

1. Crear archivo `.env` desde `.env.example` y completar valores.
2. Instalar dependencias:

```bash
cd telegram-receipt-bot
npm install
```

3. Inicializar esquema de base de datos:

```bash
npm run prisma:db:push
```

4. Levantar el bot:

```bash
npm start
```

## Nota de versionado

- Se versiona `dist/`.
- No se versiona `node_modules/`.

## Fallback AI parser (opcional)

Si el parser interno no logra extraer campos obligatorios (o devuelve baja confianza), el bot puede probar un parser AI externo.

Variables:

- `AI_PARSER_FALLBACK_ENABLED=true|false`
- `AI_PARSER_BASE_URL` (ej: `https://tu-parser.up.railway.app`)
- `AI_PARSER_PARSE_PATH` (default: `/parse`)
- `AI_PARSER_API_KEY` (opcional)
- `AI_PARSER_API_KEY_HEADER` (default: `X-API-Key`)
- `AI_PARSER_TIMEOUT_MS` (default: `8000`)
- `AI_PARSER_MIN_CONFIDENCE` (default: `0.55`)
