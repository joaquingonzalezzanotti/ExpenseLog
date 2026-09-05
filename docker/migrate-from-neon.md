# Migración desde Neon → Homeserver

Guía paso a paso para importar los datos de producción (Neon) al PostgreSQL local del homeserver.

## Prerrequisitos

- Stack del homeserver corriendo con PostgreSQL vacío (`docker compose up -d postgres`)
- Credenciales de Neon (host, user, password, dbname)
- `pg_dump` instalado localmente **o** usar el contenedor de backup

---

## Paso 1: Dump desde Neon

Desde tu máquina local (necesitás `pg_dump` v17 o compatible):

```bash
# Base principal de ExpenseLog
pg_dump -Fc \
  "postgresql://USER:PASS@HOST/expenselog?sslmode=require" \
  > expenselog-neon.dump

# Base del bot de Telegram (si la usás)
pg_dump -Fc \
  "postgresql://USER:PASS@HOST/expenselog_bot?sslmode=require" \
  > expenselog_bot-neon.dump
```

> **Tip**: Reemplazá `USER`, `PASS`, `HOST` con las credenciales de tu proyecto en Neon.
> Las encontrás en el dashboard de Neon → tu proyecto → Connection Details.

---

## Paso 2: Copiar dumps al homeserver

```bash
scp expenselog-neon.dump expenselog_bot-neon.dump user@homeserver:/ruta/al/repo/docker/
```

O si estás trabajando directamente en el homeserver, simplemente movelos al directorio `docker/`.

---

## Paso 3: Restaurar en PostgreSQL local

Usamos el contenedor de backup que ya tiene `pg_restore`:

```bash
# Asegurate de que PostgreSQL esté corriendo
docker compose -f docker/compose.yaml up -d postgres

# Esperar a que esté healthy
docker compose -f docker/compose.yaml exec postgres pg_isready -U expenselog

# Restaurar base principal
docker compose -f docker/compose.yaml --profile tools run --rm \
  -v $(pwd)/docker:/dumps \
  backup pg_restore --clean --if-exists \
    -h postgres -U expenselog -d expenselog /dumps/expenselog-neon.dump

# Restaurar base del bot (si aplica)
docker compose -f docker/compose.yaml --profile tools run --rm \
  -v $(pwd)/docker:/dumps \
  backup pg_restore --clean --if-exists \
    -h postgres -U expenselog -d expenselog_bot /dumps/expenselog_bot-neon.dump
```

---

## Paso 4: Inicializar Prisma (si levantás el bot)

```bash
docker compose --profile bot run --rm telegram-bot npx prisma db push
```

---

## Paso 5: Verificar datos

```bash
# Contar usuarios y gastos
docker compose exec postgres psql -U expenselog -d expenselog \
  -c "SELECT 'users' AS t, COUNT(*) FROM users UNION ALL SELECT 'expenses', COUNT(*) FROM expenses UNION ALL SELECT 'recurring', COUNT(*) FROM recurring_expenses;"

# Verificar base del bot
docker compose exec postgres psql -U expenselog -d expenselog_bot \
  -c "SELECT 'telegram_user_link' AS t, COUNT(*) FROM telegram_user_link UNION ALL SELECT 'receipt_draft', COUNT(*) FROM receipt_draft;"
```

---

## Paso 6: Levantar todo y probar

```bash
# Levantar stack completo
docker compose -f docker/compose.yaml --profile bot --profile parser up -d

# Verificar que todo está healthy
docker compose -f docker/compose.yaml ps

# Probar el endpoint
curl https://me.expenselog.com.ar/health
```

---

## Paso 7: Primer backup post-migración

```bash
docker compose -f docker/compose.yaml --profile tools run --rm backup
```

---

## Notas importantes

- **No uses las dos instancias en paralelo para escribir datos**. Una vez que hagas el dump de Neon, la instancia personal es tu nueva fuente de verdad.
- **Railway sigue funcionando** con su propia Neon. No se toca nada de producción.
- Si algo sale mal, podés repetir el dump/restore cuantas veces necesites.
