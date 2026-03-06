# Bot Integration Guide

## Objetivo

Definir como `bot-prod` usa este parser:
- texto -> parser directo,
- imagen/pdf -> parser interno del bot primero,
- fallback a `ai-parser` si parser interno falla o baja confianza.

## Contrato recomendado

Endpoint: `POST /api/parse`

Headers:
- `Content-Type: application/json`
- `X-Internal-Token: <INTERNAL_TOKEN>` (si esta configurado)

Body soportado:
- `text` (string)
- `media` (`mime_type`, `data_base64`, `filename?`)
- ambos a la vez

## Decision tree del bot

1. Mensaje texto:
   - llamar `ai-parser` directo con `text`.

2. Mensaje imagen/pdf:
   - correr parser interno del bot.
   - si parser interno `ok && confidence >= threshold`, usar ese resultado.
   - si no, fallback a `ai-parser` con `media` + contexto opcional.

3. Confirmacion usuario:
   - mostrar resumen de monto, tipo, fecha, contraparte.
   - si usuario confirma, guardar en ExpenseLog API.

## Sugerencia de threshold

- `confidence >= 0.8`: usar parser interno del bot.
- `< 0.8`: fallback a `ai-parser`.

## Recomendacion operativa

- `bot-prod`, `ExpenseLog` y `ai-parser-prod` como 3 servicios separados.
- Conexion bot -> parser por red interna.
- No exponer parser publico sin token.
