BEGIN;

-- Demo account for product screenshots
-- Login credentials:
--   email: demo.screenshots@expenselog.local
--   password: DemoShots2026!

DELETE FROM sessions WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM password_resets WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM email_verifications WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM oauth_identities WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM wallet_ingest_events WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM wallet_ingest_tokens WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM whatsapp_link_codes WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM whatsapp_user_links WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM telegram_link_codes WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM telegram_user_links WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM reconciliations WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM expenses WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM recurring_expenses WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM categories WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM user_config WHERE user_id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051';
DELETE FROM users WHERE id = '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051' OR email = 'demo.screenshots@expenselog.local';

INSERT INTO users (id, email, name, password_hash, status, created_at)
VALUES (
  '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051',
  'demo.screenshots@expenselog.local',
  'Demo Screenshots',
  '$2a$10$f1dkMRPAff4iwURzTDVNiOQiP1PnTn4czPBa9tUsqCNDLMnHYpIau',
  'active',
  NOW()
);

INSERT INTO user_config (user_id, currency, start_date, plan_tier)
VALUES ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'ars', 1, 'free');

INSERT INTO categories (user_id, name, position)
VALUES
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Comida', 1),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Supermercado', 2),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Transporte', 3),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Alquiler', 4),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Servicios', 5),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Salud', 6),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Entretenimiento', 7),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Compras', 8),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Hogar', 9),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Educacion', 10),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Viajes', 11),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Inversion', 12),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Ingresos', 13),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Varios', 14),
  ('8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051', 'Tarjeta por pagar', 15);

INSERT INTO recurring_expenses (
  id, user_id, name, amount, currency, category, start_date, interval, occurrences, flow, tags
)
VALUES
  (
    '33333333-3333-4333-8333-333333333333',
    '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051',
    'Alquiler departamento',
    -780000.00,
    'ars',
    'Alquiler',
    date_trunc('month', NOW()) - INTERVAL '10 months',
    'monthly',
    24,
    'expense',
    '["fijo","hogar"]'
  ),
  (
    '44444444-4444-4444-8444-444444444444',
    '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051',
    'Internet fibra',
    -35000.00,
    'ars',
    'Servicios',
    date_trunc('month', NOW()) - INTERVAL '10 months',
    'monthly',
    24,
    'expense',
    '["fijo","servicios"]'
  ),
  (
    '55555555-5555-4555-8555-555555555555',
    '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051',
    'Spotify',
    -5999.00,
    'ars',
    'Entretenimiento',
    date_trunc('month', NOW()) - INTERVAL '10 months',
    'monthly',
    24,
    'expense',
    '["suscripcion","streaming"]'
  ),
  (
    '66666666-6666-4666-8666-666666666666',
    '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051',
    'Seguro del auto',
    -52000.00,
    'ars',
    'Servicios',
    date_trunc('month', NOW()) - INTERVAL '10 months',
    'monthly',
    24,
    'expense',
    '["fijo","auto"]'
  ),
  (
    '77777777-7777-4777-8777-777777777777',
    '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051',
    'Sueldo',
    1850000.00,
    'ars',
    'Ingresos',
    date_trunc('month', NOW()) - INTERVAL '10 months',
    'monthly',
    24,
    'income',
    '["fijo","laboral"]'
  );

WITH seq AS (
  SELECT generate_series(1, 260) AS n
),
mock_rows AS (
  SELECT
    n,
    date_trunc('day', NOW())
      - (n || ' days')::interval
      + make_interval(hours => ((n * 7) % 14) + 8, mins => ((n * 11) % 60)) AS occurred_at,
    CASE n % 12
      WHEN 0 THEN 'Comida'
      WHEN 1 THEN 'Supermercado'
      WHEN 2 THEN 'Transporte'
      WHEN 3 THEN 'Servicios'
      WHEN 4 THEN 'Salud'
      WHEN 5 THEN 'Entretenimiento'
      WHEN 6 THEN 'Compras'
      WHEN 7 THEN 'Hogar'
      WHEN 8 THEN 'Educacion'
      WHEN 9 THEN 'Viajes'
      WHEN 10 THEN 'Inversion'
      ELSE 'Ingresos'
    END AS category,
    CASE
      WHEN n % 17 = 0 THEN 'income'
      WHEN n % 29 = 0 THEN 'refund'
      ELSE 'expense'
    END AS flow
  FROM seq
)
INSERT INTO expenses (
  id,
  user_id,
  recurring_id,
  name,
  category,
  amount,
  currency,
  date,
  flow,
  tags,
  source,
  card,
  system_origin,
  system_locked,
  created_at
)
SELECT
  lower(
    substr(md5('demo.screenshots.expense.' || n), 1, 8) || '-' ||
    substr(md5('demo.screenshots.expense.' || n), 9, 4) || '-' ||
    '4' || substr(md5('demo.screenshots.expense.' || n), 14, 3) || '-' ||
    'a' || substr(md5('demo.screenshots.expense.' || n), 18, 3) || '-' ||
    substr(md5('demo.screenshots.expense.' || n), 21, 12)
  ) AS id,
  '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051' AS user_id,
  CASE
    WHEN n % 60 = 0 THEN '33333333-3333-4333-8333-333333333333'
    WHEN n % 45 = 0 THEN '44444444-4444-4444-8444-444444444444'
    WHEN n % 90 = 0 THEN '55555555-5555-4555-8555-555555555555'
    ELSE NULL
  END AS recurring_id,
  CASE
    WHEN flow = 'income' AND n % 51 = 0 THEN 'Sueldo mensual'
    WHEN flow = 'income' THEN 'Cobro freelance'
    WHEN flow = 'refund' THEN 'Reintegro de compra'
    WHEN category = 'Comida' THEN 'Cena en restaurante'
    WHEN category = 'Supermercado' THEN 'Compra semanal'
    WHEN category = 'Transporte' THEN 'Viaje en app'
    WHEN category = 'Servicios' THEN 'Pago de servicio'
    WHEN category = 'Salud' THEN 'Farmacia'
    WHEN category = 'Entretenimiento' THEN 'Salida con amigos'
    WHEN category = 'Compras' THEN 'Compra online'
    WHEN category = 'Hogar' THEN 'Articulos para casa'
    WHEN category = 'Educacion' THEN 'Curso online'
    WHEN category = 'Viajes' THEN 'Pasaje'
    WHEN category = 'Inversion' THEN 'Compra de dolar MEP'
    ELSE 'Movimiento'
  END AS name,
  category,
  CASE
    WHEN flow = 'income' AND n % 51 = 0 THEN round((2450000 + (n % 3) * 210000)::numeric, 2)
    WHEN flow = 'income' THEN round((230000 + (n % 9) * 42000 + ((n % 5) * 1350.75))::numeric, 2)
    WHEN flow = 'refund' THEN round((15000 + (n % 15) * 4200 + ((n % 3) * 210.40))::numeric, 2)
    ELSE -round((
      CASE category
        WHEN 'Comida' THEN 14000 + ((n * 97) % 52000)
        WHEN 'Supermercado' THEN 36000 + ((n * 131) % 120000)
        WHEN 'Transporte' THEN 9000 + ((n * 89) % 34000)
        WHEN 'Servicios' THEN 23000 + ((n * 113) % 76000)
        WHEN 'Salud' THEN 17000 + ((n * 107) % 68000)
        WHEN 'Entretenimiento' THEN 13000 + ((n * 149) % 70000)
        WHEN 'Compras' THEN 30000 + ((n * 173) % 190000)
        WHEN 'Hogar' THEN 19000 + ((n * 157) % 95000)
        WHEN 'Educacion' THEN 16000 + ((n * 127) % 90000)
        WHEN 'Viajes' THEN 45000 + ((n * 199) % 280000)
        WHEN 'Inversion' THEN 120000 + ((n * 211) % 620000)
        ELSE 12000 + ((n * 97) % 55000)
      END
      + CASE WHEN n % 10 IN (0, 1) THEN 9000 ELSE 0 END
      + ((n % 7) * 45.60)
    )::numeric, 2)
  END AS amount,
  'ars' AS currency,
  occurred_at AS date,
  flow,
  to_json(ARRAY[
    lower(replace(category, ' ', '_')),
    CASE
      WHEN flow = 'expense' THEN 'egreso'
      WHEN flow = 'income' THEN 'ingreso'
      ELSE 'reintegro'
    END,
    CASE
      WHEN n % 2 = 0 THEN 'captura'
      ELSE 'demo'
    END
  ])::text AS tags,
  CASE
    WHEN flow = 'income' THEN
      CASE
        WHEN n % 3 = 0 THEN 'TRANSFERENCIA'
        WHEN n % 3 = 1 THEN 'CA'
        ELSE 'SUELDO'
      END
    WHEN flow = 'refund' THEN 'TARJETA'
    ELSE
      CASE n % 4
        WHEN 0 THEN 'CA'
        WHEN 1 THEN 'TARJETA'
        WHEN 2 THEN 'TRANSFERENCIA'
        ELSE 'DEBITO'
      END
  END AS source,
  CASE
    WHEN flow = 'expense' AND n % 4 IN (1, 3) THEN
      CASE n % 5
        WHEN 0 THEN 'Visa Santander'
        WHEN 1 THEN 'Mastercard Galicia'
        WHEN 2 THEN 'Naranja X'
        WHEN 3 THEN 'Amex Reba'
        ELSE 'Visa BBVA'
      END
    WHEN flow = 'refund' THEN
      CASE n % 3
        WHEN 0 THEN 'Visa Santander'
        WHEN 1 THEN 'Mastercard Galicia'
        ELSE 'Naranja X'
      END
    ELSE NULL
  END AS card,
  'user' AS system_origin,
  FALSE AS system_locked,
  occurred_at AS created_at
FROM mock_rows;

-- Balance tuning before focused screenshot month:
-- keeps carryover meaningful but not inflated.
WITH month_ctx AS (
  SELECT date_trunc('month', NOW()) - INTERVAL '1 month' AS focus_month_start
)
INSERT INTO expenses (
  id, user_id, recurring_id, name, category, amount, currency, date, flow, tags, source, card, system_origin, system_locked, created_at
)
SELECT
  x.id,
  '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051',
  NULL,
  x.name,
  x.category,
  x.amount,
  'ars',
  x.occurred_at,
  x.flow,
  x.tags,
  'CA',
  NULL,
  'user',
  FALSE,
  x.occurred_at
FROM month_ctx,
LATERAL (
  VALUES
    (
      '88888888-8888-4888-a888-888888888810',
      'Aporte cartera inversion',
      'Inversion',
      -3200000.00::numeric,
      focus_month_start - INTERVAL '18 days',
      'expense',
      '["inversion","capital"]'
    ),
    (
      '88888888-8888-4888-a888-888888888811',
      'Compra dolar MEP',
      'Inversion',
      -1550000.00::numeric,
      focus_month_start - INTERVAL '7 days',
      'expense',
      '["inversion","cobertura"]'
    ),
    (
      '88888888-8888-4888-a888-888888888812',
      'Rescate parcial inversion',
      'Ingresos',
      760000.00::numeric,
      focus_month_start - INTERVAL '2 days',
      'income',
      '["inversion","rescate"]'
    )
) AS x(id, name, category, amount, occurred_at, flow, tags);

-- Focused activity for the screenshot month (previous month relative to execution date):
-- dense, realistic mix to improve KPI readability.
WITH month_ctx AS (
  SELECT date_trunc('month', NOW()) - INTERVAL '1 month' AS month_start
),
focus_seq AS (
  SELECT generate_series(1, 96) AS n
),
focus_rows AS (
  SELECT
    n,
    month_start
      + (((n - 1) % 28) || ' days')::interval
      + make_interval(hours => 8 + ((n * 3) % 12), mins => ((n * 17) % 60)) AS occurred_at,
    CASE ((n - 1) % 12)
      WHEN 0 THEN 'Comida'
      WHEN 1 THEN 'Supermercado'
      WHEN 2 THEN 'Transporte'
      WHEN 3 THEN 'Servicios'
      WHEN 4 THEN 'Salud'
      WHEN 5 THEN 'Entretenimiento'
      WHEN 6 THEN 'Compras'
      WHEN 7 THEN 'Hogar'
      WHEN 8 THEN 'Educacion'
      WHEN 9 THEN 'Viajes'
      WHEN 10 THEN 'Inversion'
      ELSE 'Varios'
    END AS category
  FROM focus_seq
  CROSS JOIN month_ctx
),
focus_enriched AS (
  SELECT
    n,
    occurred_at,
    category,
    EXTRACT(ISODOW FROM occurred_at)::int AS dow,
    CASE
      WHEN n IN (1, 2) THEN 'income'
      WHEN n % 23 = 0 THEN 'refund'
      ELSE 'expense'
    END AS flow,
    CASE
      WHEN n IN (1, 2) THEN 'CA'
      WHEN n % 23 = 0 THEN 'TARJETA'
      ELSE
        CASE
          -- mostly CA so dashboard KPIs reflect operational cash usage
          WHEN n % 9 = 0 THEN 'TARJETA'
          ELSE 'CA'
        END
    END AS source
  FROM focus_rows
)
INSERT INTO expenses (
  id,
  user_id,
  recurring_id,
  name,
  category,
  amount,
  currency,
  date,
  flow,
  tags,
  source,
  card,
  system_origin,
  system_locked,
  created_at
)
SELECT
  lower(
    substr(md5('demo.screenshots.focus.' || n), 1, 8) || '-' ||
    substr(md5('demo.screenshots.focus.' || n), 9, 4) || '-' ||
    '4' || substr(md5('demo.screenshots.focus.' || n), 14, 3) || '-' ||
    'a' || substr(md5('demo.screenshots.focus.' || n), 18, 3) || '-' ||
    substr(md5('demo.screenshots.focus.' || n), 21, 12)
  ) AS id,
  '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051' AS user_id,
  NULL AS recurring_id,
  CASE
    WHEN flow = 'income' AND n = 1 THEN 'Sueldo mensual'
    WHEN flow = 'income' AND n = 2 THEN 'Bono trimestral'
    WHEN flow = 'income' THEN 'Ingreso extra'
    WHEN flow = 'refund' THEN 'Reintegro consumo tarjeta'
    WHEN category = 'Comida' THEN 'Almuerzo y cena'
    WHEN category = 'Supermercado' THEN 'Compra semanal'
    WHEN category = 'Transporte' THEN 'Combustible y peajes'
    WHEN category = 'Servicios' THEN 'Pago de servicio'
    WHEN category = 'Salud' THEN 'Farmacia y consulta'
    WHEN category = 'Entretenimiento' THEN 'Salida con amigos'
    WHEN category = 'Compras' THEN 'Compra online'
    WHEN category = 'Hogar' THEN 'Mantenimiento hogar'
    WHEN category = 'Educacion' THEN 'Curso / capacitacion'
    WHEN category = 'Viajes' THEN 'Pasaje y reserva'
    WHEN category = 'Inversion' THEN 'Aporte inversion'
    ELSE 'Gasto varios'
  END AS name,
  CASE WHEN flow = 'income' THEN 'Ingresos' ELSE category END AS category,
  CASE
    WHEN flow = 'income' AND n = 1 THEN 2650000.00::numeric
    WHEN flow = 'income' AND n = 2 THEN 620000.00::numeric
    WHEN flow = 'income' THEN round((260000 + (n % 7) * 52000 + ((n % 4) * 1800.50))::numeric, 2)
    WHEN flow = 'refund' THEN round((22000 + (n % 9) * 4800 + ((n % 3) * 260.45))::numeric, 2)
    ELSE -round((
      CASE category
        WHEN 'Comida' THEN 15000 + ((n * 97) % 48000)
        WHEN 'Supermercado' THEN 42000 + ((n * 137) % 140000)
        WHEN 'Transporte' THEN 11000 + ((n * 83) % 42000)
        WHEN 'Servicios' THEN 26000 + ((n * 109) % 90000)
        WHEN 'Salud' THEN 18000 + ((n * 101) % 78000)
        WHEN 'Entretenimiento' THEN 14000 + ((n * 151) % 82000)
        WHEN 'Compras' THEN 36000 + ((n * 179) % 220000)
        WHEN 'Hogar' THEN 22000 + ((n * 163) % 110000)
        WHEN 'Educacion' THEN 17000 + ((n * 127) % 95000)
        WHEN 'Viajes' THEN 58000 + ((n * 197) % 320000)
        WHEN 'Inversion' THEN 140000 + ((n * 223) % 700000)
        ELSE 14000 + ((n * 97) % 60000)
      END
      + (CASE WHEN dow IN (5, 6, 7) THEN 14000 ELSE 0 END)
      + (CASE WHEN category = 'Viajes' THEN 25000 ELSE 0 END)
      + (CASE WHEN category = 'Compras' THEN 12000 ELSE 0 END)
    )::numeric, 2)
  END AS amount,
  'ars' AS currency,
  occurred_at AS date,
  flow,
  to_json(ARRAY[
    lower(replace(category, ' ', '_')),
    CASE
      WHEN flow = 'expense' THEN 'egreso'
      WHEN flow = 'income' THEN 'ingreso'
      ELSE 'reintegro'
    END,
    'captura'
  ])::text AS tags,
  source,
  CASE
    WHEN source = 'TARJETA' THEN
      CASE n % 6
        WHEN 0 THEN 'Visa Santander'
        WHEN 1 THEN 'Mastercard Galicia'
        WHEN 2 THEN 'Naranja X'
        WHEN 3 THEN 'Amex Reba'
        WHEN 4 THEN 'Visa BBVA'
        ELSE 'Mastercard ICBC'
      END
    ELSE NULL
  END AS card,
  'user' AS system_origin,
  FALSE AS system_locked,
  occurred_at AS created_at
FROM focus_enriched;

-- Card payments recorded in the same focused month
-- so "Tarjeta por pagar" remains realistic (not just ever-growing debt).
WITH month_ctx AS (
  SELECT date_trunc('month', NOW()) - INTERVAL '1 month' AS month_start
)
INSERT INTO expenses (
  id, user_id, recurring_id, name, category, amount, currency, date, flow, tags, source, card, system_origin, system_locked, created_at
)
SELECT
  x.id,
  '8f97c6d8-0f56-4eb2-97f6-4cf95d5c1051',
  NULL,
  x.name,
  'Tarjeta por pagar',
  x.amount,
  'ars',
  x.occurred_at,
  'expense',
  '["tarjeta","pago"]',
  'CA',
  x.card,
  'card_payment_owner',
  FALSE,
  x.occurred_at
FROM month_ctx,
LATERAL (
  VALUES
    (
      '88888888-8888-4888-a888-888888888821',
      'Pago tarjeta - Visa Santander',
      -185000.00::numeric,
      month_start + INTERVAL '16 days',
      'Visa Santander'
    ),
    (
      '88888888-8888-4888-a888-888888888822',
      'Pago tarjeta - Mastercard Galicia',
      -120000.00::numeric,
      month_start + INTERVAL '25 days',
      'Mastercard Galicia'
    )
) AS x(id, name, amount, occurred_at, card);

COMMIT;
