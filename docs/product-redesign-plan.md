# Plan de Producto y Redisenio (Base de Trabajo)

## Objetivo
Documentar en un solo lugar las decisiones de producto, arquitectura y UX para:

1. No perder contexto durante el redisenio.
2. Evitar cambios contradictorios entre sprints.
3. Ejecutar por fases con criterios de aceptacion claros.

Este documento es la fuente de verdad para avanzar en `dev`.

## Estado del producto (contexto de arranque)

1. No hay usuarios activos externos (solo uso interno).
2. Es el momento ideal para cambios estructurales.
3. Prioridad #1 acordada: reorganizar rutas y arquitectura web.

## Estado de implementacion en dev (22 Feb 2026)

1. Ya existe endpoint `POST /api/card/payment` para registrar pagos de tarjeta.
2. Se guardan dos variantes:
   - `paidBy=self` -> movimiento en `CA` con `systemOrigin=card_payment_owner`.
   - `paidBy=third_party` -> movimiento en `TARJETA` con `systemOrigin=card_payment_third_party`.
3. Home (`/app`) ya muestra modulo `Tarjeta por pagar` con:
   - saldo pendiente por moneda,
   - boton `Pagar tarjeta`,
   - boton `Pago por tercero`.
4. El pendiente de tarjeta considera:
   - consumos en `TARJETA`,
   - reintegros en `TARJETA`,
   - pagos propios registrados por el nuevo endpoint.
5. Formulario principal de movimientos rediseniado en modal (desktop/mobile) con:
   - layout mas compacto,
   - copy de impacto por metodo,
   - estados visuales consistentes con el resto del dashboard.
6. Modulo `Tarjeta por pagar` mejorado en UX:
   - estado vacio visual (`is-empty`),
   - moneda de pago auto-seleccionada segun pendiente disponible,
   - selector de moneda simplificado para evitar overflow.
7. Flujo de cuotas/recurrentes visible en UI:
   - tabla muestra badge `Cuota X/Y` cuando hay `recurringID`,
   - calendario financiero muestra `Cuota X/Y` en el detalle diario,
   - home (`Ultimos 3 movimientos`) incluye sufijo de cuota.
8. Copy visible en UI limpiado para evitar `CA` en textos de usuario:
   - dashboard (`Saldo inicial`),
   - settings (conciliacion de balance),
   - mensajes de ayuda y estados.

## Prioridad #1 (cerrada): Arquitectura de rutas

### Objetivo
Separar claramente:

1. Sitio publico.
2. Aplicacion autenticada.
3. API.

### Namespace objetivo

1. `https://expenselog.com.ar/` -> Landing publica.
2. `https://expenselog.com.ar/app/*` -> Aplicacion.
3. `https://expenselog.com.ar/api/*` -> API backend.

### Estado tecnico (22 Feb 2026)

1. Backend ya expone rutas nuevas `/app/*` y `/api/*`.
2. Frontend de app consume assets bajo `/app/*`.
3. Se mantiene compatibilidad temporal con rutas legacy para no romper despliegues durante la migracion.

### Motivo

1. Mejor escalabilidad de arquitectura.
2. Menos friccion para separar landing de app sin romper UX.
3. URLs mas consistentes para usuario y para operaciones.

## Decisiones funcionales cerradas (hasta hoy)

### KPI y lenguaje de UI

1. Evitar terminos tecnicos en UI principal.
2. Mantener `CA` como termino interno si hace falta, pero no como texto principal visible.
3. Set final de KPI acordado:
   - `Ingresos`
   - `Egresos`
   - `Balance` (equivale a disponible actual)
   - `Tarjeta por pagar`

### Medios para gastos (modelo funcional)

1. `Transferencia` -> impacta `Disponible ahora`.
2. `Tarjeta debito` -> impacta `Disponible ahora`.
3. `Tarjeta credito` -> no impacta `Disponible ahora` al comprar.
4. `Efectivo` -> en MVP funciona como log (sin impactar `Disponible ahora`).

### Tarjeta de credito (regla contable - base caja)

1. La compra en credito incrementa `Tarjeta por pagar`.
2. La compra en credito no impacta `Balance` al momento de compra.
3. La compra en credito no impacta `Egresos` al momento de compra.
4. El egreso se reconoce cuando se paga (si paga el titular).

### Pago de tarjeta (obligatorio para MVP nuevo)

1. Debe existir evento explicito de pago.
2. Debe bajar `Tarjeta por pagar`.
3. Si paga el titular:
   - baja `Balance`
   - sube `Egresos`
4. Si paga tercero:
   - no cambia `Balance`
   - no cambia `Egresos`

### Pago por tercero (caso especial requerido)

1. Debe existir accion para marcar pago de tarjeta sin descontar balance propio.
2. Debe bajar `Tarjeta por pagar`.
3. No debe tocar `Balance`.
4. No debe sumar gasto.

## Cuotas (enfoque acordado)

### Estado actual utilizable

1. Ya existe logica via transaccion recurrente mensual.
2. `repeticiones = cantidad de cuotas`.

### Decision de UX

1. Exponerlo como flujo de `Compra en cuotas` (no como concepto tecnico de recurrentes).
2. Crear la recurrencia por detras.
3. Mostrar en tabla contador de cuota: `X/Y`.

### Contador propuesto

1. `Y` = total de repeticiones de la regla.
2. `X` = indice de ocurrencia de esa transaccion en su `recurringID`.

## Redisenio del formulario de movimientos

### Problema
El formulario actual no comunica bien el impacto de cada medio en KPIs.

### Objetivo
Que el usuario entienda el impacto antes de guardar.

### Requisitos funcionales UX

1. Campo `Como lo pagaste` solo en gasto.
2. Mensaje dinamico por medio:
   - Transferencia/Debito: impacta balance.
   - Credito: no impacta balance, suma tarjeta por pagar.
   - Efectivo: solo registro (MVP).
3. Preview de impacto antes de guardar:
   - Balance antes.
   - Balance despues.
4. En ingreso no mostrar tarjeta credito como medio.

## Backlog por fases (orden recomendado)

### Fase A - Congelar contrato funcional

1. Definir oficialmente:
   - Tipos de movimiento.
   - Reglas de KPIs.
   - Reglas de pago de tarjeta normal vs pago por tercero.
2. Escribir ADR corto de reglas contables.

### Fase B - Refactor de rutas (P0)

1. Mover frontend app a `/app/*`.
2. Mover API a `/api/*`.
3. Agregar redirects legacy (compatibilidad temporal).
4. Ajustar service worker/manifest/scope.

### Fase C - Formulario nuevo y copy de impacto

1. Redisenio de layout y jerarquia.
2. Mensajes explicativos por medio.
3. Preview de impacto en KPI.

### Fase D - Tarjeta credito completa

1. Crear evento `Pagar tarjeta`.
2. Crear accion `Marcar pago por tercero`.
3. Reflejar ambos en KPIs y tabla.

### Fase E - Cuotas UX

1. Crear flujo `Compra en cuotas`.
2. Persistir via recurrencias (motor existente).
3. Mostrar `Cuota X/Y` en tabla y detalles.

### Fase F - Documentacion externa y ayuda in-app

1. Landing: seccion "Como calcula la app".
2. FAQ: casos reales (credito, pago, tercero, efectivo).
3. Tooltips contextuales en formulario y modulos KPI.

## Casos de prueba (aceptacion funcional)

### Caso 1: Gasto por transferencia

1. Cargo gasto 10.000 por transferencia.
2. Esperado:
   - Egresos +10.000.
   - Balance -10.000.
   - Tarjeta por pagar sin cambios.

### Caso 2: Gasto por debito

1. Cargo gasto 5.000 por debito.
2. Esperado:
   - Egresos +5.000.
   - Balance -5.000.
   - Tarjeta por pagar sin cambios.

### Caso 3: Compra con tarjeta credito

1. Cargo gasto 20.000 con tarjeta credito.
2. Esperado:
   - Egresos sin cambios.
   - Balance sin cambios.
   - Tarjeta por pagar +20.000.

### Caso 4: Pago de tarjeta propio

1. Pago 12.000 de tarjeta desde disponible.
2. Esperado:
   - Egresos +12.000.
   - Balance -12.000.
   - Tarjeta por pagar -12.000.

### Caso 5: Pago de tarjeta por tercero

1. Marco pago por tercero por 8.000.
2. Esperado:
   - Egresos sin cambios.
   - Balance sin cambios.
   - Tarjeta por pagar -8.000.

### Caso 6: Gasto en efectivo (MVP simple)

1. Cargo gasto 3.000 en efectivo.
2. Esperado:
   - Egresos +3.000.
   - Balance sin cambios.
   - Tarjeta por pagar sin cambios.

### Caso 7: Compra en cuotas

1. Cargo compra de 6 cuotas de 15.000.
2. Esperado:
   - Se crea regla mensual de 6 repeticiones.
   - En tabla aparece cuota actual `1/6`, luego `2/6`, etc.
   - Cada cuota incrementa `Tarjeta por pagar` y no impacta `Egresos/Balance` hasta su pago.

## Riesgos y mitigaciones

### Riesgo 1: Inconsistencia entre KPI y tabla

1. Mitigacion: reglas unificadas en capa de dominio + tests de regresion.

### Riesgo 2: Romper rutas existentes al migrar a `/app` y `/api`

1. Mitigacion: fase de compatibilidad con redirects y smoke tests.

### Riesgo 3: Confusion de usuario por cambio de terminos

1. Mitigacion: copy consistente en formulario, KPI, landing y FAQ.

## Definition of Done global (este ciclo)

1. Rutas activas y estables: `/`, `/app/*`, `/api/*`.
2. Formulario nuevo comunica impacto por medio.
3. Pago de tarjeta normal implementado.
4. Pago por tercero implementado.
5. Compra en cuotas via flujo dedicado implementada.
6. Tabla muestra `Cuota X/Y`.
7. Casos de prueba funcionales verificados.
8. Documentacion de producto y FAQ alineada al comportamiento real.

## Notas de alcance

1. Este ciclo no incluye aun modulo avanzado de seguimiento de efectivo.
2. Este ciclo no incluye aun expansion a ahorro/budget.
3. Esos temas se retoman despues de estabilizar este redisenio base.
