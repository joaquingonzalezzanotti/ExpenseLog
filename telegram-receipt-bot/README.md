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
