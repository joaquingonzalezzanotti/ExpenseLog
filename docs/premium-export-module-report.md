# Reporte: Modulo Premium de Exportacion Mensual (Excel + PDF)

Fecha: 22 Feb 2026  
Rama: `onboarding`

## 1) Objetivo
Definir como implementar un modulo de exportacion de movimientos mensuales en formato Excel y PDF, bloqueado para usuarios Free y habilitado para usuarios Premium.

## 2) Hallazgos de investigacion (benchmark)

### 2.1 Lo que hacen otras apps
1. Wallet (BudgetBakers) marca la exportacion como feature premium y permite exportar por fecha/filtros con formatos `PDF/XLS/CSV`.
2. Spendee limita export en plan free (365 dias) y deja export sin limite en planes pagos (Plus/Premium), con formatos `CSV/XLS`.
3. Toshl modela export como proceso asincrono con estado (`generating`, `generated`) y filtros avanzados; soporta `csv/xls/pdf/ofx`.
4. Monarch permite descarga CSV de transacciones/balances, con limite operativo recomendado (<=5k por descarga aunque soporte hasta 10k).
5. EveryDollar explicita que `Export data` forma parte de su plan Premium.

### 2.2 Patrones comunes
1. Export se ofrece como valor premium.
2. Export siempre tiene filtros por rango de fechas.
3. Formato tabular para analisis (CSV/XLS) + formato visual para compartir (PDF).
4. Para volumen alto, conviene asincronia y/o particionado por lotes.

## 3) Estado actual de ExpenseLog (repo)
1. Existen endpoints legacy de import/export CSV pero hoy estan deshabilitados:
   - `POST/GET /api/export/csv` y `/api/import/csv` responden "deshabilitada temporalmente".
2. No existe hoy modulo de suscripciones/planes en backend.
3. No existe UI activa de export en `settings`/`table`.
4. El modelo de datos ya tiene la informacion necesaria para export:
   - `flow`, `source`, `card`, `currency`, `category`, `tags`, `date`, `amount`, `systemOrigin`.

## 4) Definicion de producto propuesta

### 4.1 Regla comercial (premium por defecto)
1. El modulo `Exportacion mensual` se publica como `Premium`.
2. Usuarios Free:
   - ven la opcion en UI (teaser/upsell),
   - no pueden ejecutar export real.
3. Usuarios Premium:
   - pueden exportar Excel y PDF del mes seleccionado.

### 4.2 Alcance funcional MVP
1. Export mensual por moneda y periodo (`year`, `month`).
2. Formatos:
   - Excel (`.xlsx`)
   - PDF (`.pdf`)
3. Filtros MVP:
   - mes/anio
   - moneda
   - opcion "incluir solo movimientos visibles" (excluye system locks si aplica)
4. Zona UI sugerida:
   - `settings` > nueva seccion `Datos y exportacion`
   - acceso rapido secundario en `table`.

### 4.3 Contenido de los archivos
1. Excel:
   - Hoja `Movimientos`: detalle fila a fila.
   - Hoja `Resumen`: KPI del periodo (Ingresos, Egresos, Balance, Tarjeta por pagar).
   - Hoja `Categorias`: agregados por categoria (monto y cantidad).
2. PDF:
   - Portada corta (usuario + periodo + moneda + fecha de emision).
   - Resumen KPI del mes.
   - Tabla compacta de movimientos.
   - Totales por categoria.

## 5) Arquitectura tecnica propuesta

### 5.1 Modelo de plan
1. Agregar plan de cuenta en `user_config`:
   - `plan_tier` (`free`, `premium`) default `free`.
2. Middleware de autorizacion de feature:
   - `RequirePremiumFeature("monthly_export")`.

### 5.2 Endpoints nuevos
1. `GET /api/export/monthly/xlsx?year=2026&month=2&currency=ars`
2. `GET /api/export/monthly/pdf?year=2026&month=2&currency=ars`
3. Respuestas:
   - `200` stream del archivo,
   - `402` o `403` si plan free (recomendado `403` + payload `feature=premium_required`),
   - `422` por parametros invalidos.

### 5.3 Generacion de archivos (Go)
1. Excel: usar `excelize` (streaming API disponible para volumen alto).
2. PDF: usar `gofpdf` (`phpdave11/gofpdf`) para generar layout server-side.

### 5.4 Estrategia de ejecucion
1. V1 (rapida): generacion sincrona y descarga directa.
2. V2 (si escala): cola asincrona + estado + link temporal (patron Toshl).

### 5.5 Seguridad y operacion
1. Requerir sesion valida (`RequireAuth`).
2. Limitar frecuencia: ejemplo 10 exports por hora por usuario.
3. Log de auditoria:
   - usuario, periodo, formato, timestamp, estado.
4. Cabeceras:
   - `Content-Disposition: attachment; filename=...`
   - `Cache-Control: no-store`.

## 6) UX y copy recomendados
1. Botones:
   - `Exportar Excel`
   - `Exportar PDF`
2. Free state:
   - "Disponible en Premium: exporta tus movimientos mensuales en Excel y PDF."
3. Carga:
   - loader corto y feedback de exito/error.
4. No mezclar con import en MVP para evitar confusion (modulos separados).

## 7) Backlog de implementacion (ejecutable)

### Fase A - Base premium (P0)
1. Migracion DB: agregar `plan_tier` a `user_config`.
2. Exponer plan en `GET /api/config`.
3. Crear helper backend `isPremium(userID)`.
4. Crear middleware `RequirePremiumFeature`.

### Fase B - Export mensual XLSX (P0)
1. Endpoint `/api/export/monthly/xlsx`.
2. Filtrado por mes/anio/moneda.
3. Generacion archivo con 3 hojas (`Movimientos`, `Resumen`, `Categorias`).
4. Tests de formato y contenido minimo.

### Fase C - Export mensual PDF (P0)
1. Endpoint `/api/export/monthly/pdf`.
2. Layout de resumen + tabla de movimientos + totales por categoria.
3. Tests basicos (status, cabeceras, archivo no vacio).

### Fase D - UI + upsell premium (P0)
1. Nueva seccion en settings: `Datos y exportacion`.
2. Si `free`: botones bloqueados + CTA premium.
3. Si `premium`: botones activos y descarga real.
4. Copys y mensajes consistentes.

### Fase E - Hardening (P1)
1. Rate limit de export.
2. Registro de auditoria.
3. Manejo de errores amigable.
4. QA responsive + smoke de descarga en desktop/mobile web.

## 8) Criterios de aceptacion (DoD)
1. Usuario Premium puede descargar XLSX y PDF del mes seleccionado.
2. Usuario Free ve el modulo pero no puede descargar.
3. Datos exportados coinciden con tabla del periodo (sin desfasajes).
4. Descargas funcionan en `/app` bajo dominio final.
5. Tests backend pasan y smoke QA documentado.

## 9) Riesgos y mitigaciones
1. Riesgo: inconsistencia entre KPI on-screen y resumen exportado.
   - Mitigacion: reutilizar mismas funciones de agregacion.
2. Riesgo: timeout en meses con muchos movimientos.
   - Mitigacion: V1 limite razonable + V2 asincrona.
3. Riesgo: confusion entre "gasto credito" y "egreso real".
   - Mitigacion: incluir nota de interpretacion en PDF/Excel.

## 10) Decisiones pendientes para cerrar alcance
1. Politica final de bloqueo Free (`403` o soft-download de muestra).
2. Si "pago por tercero" se marca con etiqueta dedicada en export.
3. Si incluir o excluir movimientos de sistema por defecto.
4. Naming comercial del plan (`Premium`, `Pro`, etc.).

## 11) Fuentes
1. Wallet Help Center - export premium y formatos PDF/XLS/CSV:  
   https://support.budgetbakers.com/hc/en-us/articles/7151606064018-How-to-export-transactions-from-Wallet
2. Spendee Help - limites free vs planes pagos para export:  
   https://help.spendee.com/article/137-export-transactions
3. Toshl Developer Docs - export asincrono y estados/formats:  
   https://developer.toshl.com/docs/exports/
4. Monarch Help - descarga de historial y recomendaciones de volumen:  
   https://help.monarchmoney.com/hc/en-us/articles/15526600975764-Download-your-transaction-history
5. EveryDollar Help - comparativa free vs premium incluyendo export:  
   https://everydollar.help.ramseysolutions.com/hc/en-us/articles/360038329811-Switch-to-or-from-EveryDollar-Premium
6. Excelize docs - soporte XLSX + streaming en Go:  
   https://xuri.me/excelize/en/
7. Excelize repo (qax-os/excelize):  
   https://github.com/qax-os/excelize
8. gofpdf repo (fork activo):  
   https://github.com/phpdave11/gofpdf
