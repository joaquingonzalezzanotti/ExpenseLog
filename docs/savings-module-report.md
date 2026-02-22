# Informe: Posibilidad de crear modulo de Ahorros

Fecha: 22 Feb 2026  
Rama: `onboarding`

## 1) Resumen ejecutivo
Si, es totalmente viable crear un modulo de ahorros en ExpenseLog sin romper el modelo actual, siempre que se modele como **transferencia interna de fondos** y no como gasto de consumo.

Conclusion recomendada:
1. Implementar MVP de `Metas de ahorro` (por objetivo, monto, fecha objetivo, progreso).
2. Registrar `aportes` y `retiros` como movimientos internos del modulo (no como egresos).
3. Mantener KPI actuales y agregar uno nuevo: `Ahorro reservado`.

## 2) Hallazgos de benchmark

### 2.1 Wallet (BudgetBakers)
Patron observado:
1. Metas con nombre, monto objetivo, monto ya ahorrado y fecha deseada.
2. Enfatiza que la app no mueve dinero automaticamente; el usuario debe moverlo a su cuenta de ahorro.
3. Buen encaje para MVP manual.

Fuente:
1. https://support.budgetbakers.com/hc/en-us/articles/7181571852690-Setting-up-Goals

### 2.2 Monarch Money (Goals 3.0)
Patron observado:
1. Separa `Save up goals` y `Pay down goals`.
2. Introduce asignaciones de fondos (fund allocations) para mover dinero dentro/fuera de metas.
3. Agrega modo "gastar reduce progreso" para metas tipo fondo de emergencia.
4. Modelo mas robusto para evolucion futura.

Fuentes:
1. https://help.monarch.com/hc/en-us/articles/15000751305108-Using-Goals
2. https://help.monarch.com/hc/en-us/articles/44373110771860-Introducing-Goals-3-0

### 2.3 YNAB (Targets)
Patron observado:
1. Objetivos por fecha, por monto, y tipo refill/top-up.
2. Excelente en calcular "cuanto falta por mes" para llegar a objetivo.
3. Muy buena referencia para recomendaciones automaticas de ahorro mensual.

Fuentes:
1. https://www.ynab.com/features/goal-tracking
2. https://www.ynab.com/blog/ynab-targets

### 2.4 Revolut (Vaults/Pockets)
Patron observado:
1. "Bolsillos" de ahorro con objetivo y fecha.
2. Ahorro recurrente y round-up (redondeos) para automatizar aporte.
3. Enfoque de UX simple, muy entendible para usuario masivo.

Fuentes:
1. https://www.revolut.com/en-US/vaults/
2. https://www.revolut.com/blog/post/meet-pockets-the-next-evolution-of-vaults/

### 2.5 Goodbudget (envelope/goal)
Patron observado:
1. Sobres anuales y sobres por objetivo para gastos/ahorros.
2. Logica de separar dinero por proposito antes de gastarlo.
3. Conceptualmente util para "ahorro por objetivo" en ExpenseLog.

Fuentes:
1. https://goodbudget.com/how-it-works/
2. https://goodbudget.com/help/getting-started-guide/step-1-add-envelopes/

## 3) Encaje con ExpenseLog hoy

## 3.1 Lo que ya existe y ayuda
1. Modelo unificado de movimientos (`expenses`) con `flow`, `currency`, `source`, `date`.
2. KPIs ya claros: `Ingresos`, `Egresos`, `Balance`, `Tarjeta por pagar`.
3. Reglas de credito ya separadas del disponible.
4. Arquitectura `/app` + `/api` facilita sumar endpoints de modulo nuevo.

## 3.2 Gap actual
1. No hay entidad de "meta de ahorro".
2. No hay concepto de "fondos reservados".
3. Si hoy el usuario registra ahorro como gasto, se distorsiona `Egresos`.

## 4) Decision contable recomendada

Regla principal:
1. **Aportar a ahorro NO es egreso de consumo**.
2. Debe bajar `Balance` disponible (dinero para usar hoy), pero no incrementar `Egresos`.
3. Debe aumentar `Ahorro reservado`.

Regla de retiro:
1. Retirar desde ahorro sube `Balance`.
2. No toca `Ingresos`.
3. Reduce `Ahorro reservado`.

Esto evita doble conteo y mantiene coherencia con tu enfoque caja.

## 5) Diseño de producto propuesto

### 5.1 MVP (simple y potente)
1. Seccion nueva: `Ahorros` en `/app/settings` y acceso rapido en home.
2. Crear meta:
   - Nombre
   - Monto objetivo
   - Moneda
   - Fecha objetivo opcional
3. Acciones:
   - `Aportar`
   - `Retirar`
4. Vista:
   - progreso visual
   - faltante
   - sugerencia mensual para llegar a tiempo

### 5.2 Flujo UX recomendado
1. Boton `Crear meta de ahorro`.
2. En cada meta: CTA principal `Aportar`.
3. Modal de aporte con opciones:
   - aporte manual
   - aporte rapido (monto sugerido del mes)
4. Mensaje claro:
   - "Este movimiento no se suma a Egresos. Solo reserva dinero."

### 5.3 Estados vacios
1. "Todavia no creaste metas. Crea tu primer objetivo: fondo de emergencia, viaje o dolar ahorro."

## 6) Propuesta tecnica

### 6.1 Nuevas tablas
1. `savings_goals`
   - `id`, `user_id`, `name`, `target_amount`, `currency`, `target_date`, `created_at`, `status`
2. `savings_allocations`
   - `id`, `user_id`, `goal_id`, `type` (`contribution`, `withdrawal`, `adjustment`), `amount`, `currency`, `date`, `note`, `created_at`

### 6.2 Endpoints sugeridos
1. `GET /api/savings/goals`
2. `POST /api/savings/goals`
3. `PUT /api/savings/goals/:id`
4. `POST /api/savings/goals/:id/contribute`
5. `POST /api/savings/goals/:id/withdraw`
6. `GET /api/savings/summary`

### 6.3 Integracion con KPIs
1. `Balance` = logica actual - aportes ahorro + retiros ahorro.
2. `Egresos` = solo consumo real (sin aportes a ahorro).
3. KPI nuevo:
   - `Ahorro reservado` (por moneda).

## 7) Riesgos clave y mitigaciones
1. Riesgo: confusion usuario entre "gasto" y "ahorro".
   - Mitigacion: copy fuerte en formulario + FAQ + tooltip.
2. Riesgo: inconsistencia KPI vs tabla.
   - Mitigacion: reglas unificadas en backend + tests de regresion.
3. Riesgo: complejidad por multimoneda.
   - Mitigacion: MVP sin conversion automatica; cada meta en su moneda.

## 8) Backlog propuesto (ejecutable)

### Fase A - Contrato funcional (P0)
1. Definir reglas contables finales de ahorro.
2. Documentar ADR corto.

### Fase B - Backend base (P0)
1. Migraciones DB (`savings_goals`, `savings_allocations`).
2. Endpoints CRUD y movimientos de aporte/retiro.
3. Tests unitarios de reglas de impacto en KPI.

### Fase C - UI MVP (P0)
1. Vista de metas + progreso.
2. Modal aportar/retirar.
3. Empty states y copy de impacto.

### Fase D - Integracion KPI (P0)
1. Agregar `Ahorro reservado` en home.
2. Ajustar calculo de `Balance` con aportes/retiros.

### Fase E - Mejora UX (P1)
1. Recordatorio de aporte mensual.
2. Sugerencia automatica "te faltan X por mes".
3. Dashboard de avance por meta.

### Fase F - Avanzado (P2)
1. Round-up opcional.
2. Aportes recurrentes automaticos.
3. Meta compartida (en caso multiusuario futuro).

## 9) Criterios de aceptacion (DoD)
1. Usuario puede crear meta y mover fondos (aportar/retirar).
2. Aportes afectan `Balance` pero no `Egresos`.
3. `Ahorro reservado` muestra valor correcto por moneda.
4. Tabla y KPI no muestran inconsistencias.
5. QA manual completa en mobile + desktop.

## 10) Recomendacion final
No mezclar ahorro con budget en esta primera etapa.  
Primero lanzar `Metas de ahorro` como modulo independiente y simple.  
Cuando este estable, recien ahi evaluar un modulo de presupuesto integrado.

