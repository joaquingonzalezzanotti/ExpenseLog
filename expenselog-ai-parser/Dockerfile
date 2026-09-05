# ── Build stage ──────────────────────────────────────────────
FROM node:20-alpine AS builder

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

COPY tsconfig.json server.ts test-parser.ts metadata.json ./
RUN npm run build

# ── Runtime stage ────────────────────────────────────────────
FROM node:20-alpine

LABEL org.opencontainers.image.source="https://github.com/joaquingonzalezzanotti/ExpenseLog"
LABEL org.opencontainers.image.description="ExpenseLog AI Parser – Gemini receipt parsing"

RUN addgroup -g 10001 -S appgroup \
    && adduser  -u 10001 -S appuser -G appgroup \
    && apk add --no-cache wget

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev && npm cache clean --force

COPY --from=builder /app/dist ./dist
COPY metadata.json ./

USER appuser
EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:3000/healthz || exit 1

CMD ["node", "dist/server.js"]
