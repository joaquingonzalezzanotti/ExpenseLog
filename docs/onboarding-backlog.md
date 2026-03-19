# Backlog de Onboarding (Rama `onboarding`)

## Objetivo
Convertir los primeros 10 minutos de uso en una experiencia guiada, clara y accionable para usuarios nuevos.

## Estado (7 Mar 2026)
1. Implementacion inicial comenzada en `internal/web/templates/index.html`, `table.html` y `style.css`.
2. Ya existe:
   - guia de 3 pasos para primer usuario en Home,
   - empty states con CTAs en Home/Tabla/Calendario,
   - ayuda contextual en `Tarjeta por pagar` sin saldo.
3. Refactor de arquitectura aplicado en `onboarding`:
   - logica extraida a `internal/web/templates/onboarding_ui.js`,
   - estilos extraidos a `internal/web/templates/onboarding.css`,
   - `index.html` y `table.html` quedan solo con integracion.
4. Pendiente:
   - QA manual completo por breakpoints,
   - ajuste fino de copy y orden de CTAs segun feedback de uso real.

## Estado operativo (19 Mar 2026)
1. Sprint 1 iniciado en rama `onboarding`.
2. `dev` ya fue integrado en `onboarding` antes de continuar desarrollo (flujo de ramas aplicado).
3. Se definio checklist ejecutable de sprint en:
   - `docs/sprints/onboarding-sprint-1-execution-2026-03-19.md`
4. Objetivo inmediato:
   - cerrar QA + copy,
   - preparar decision de merge `onboarding -> dev`.

## Rama de trabajo
1. Pasar a `onboarding`.
2. Implementar todo el alcance de este documento en esa rama.
3. Dejar listo para PR a `dev` con QA completo.

## Alcance (P0)
1. Flujo guiado real para primer usuario.
2. Empty states utiles en dashboard, tabla y tarjeta por pagar.
3. Copy simple y consistente en formularios, filtros, KPI y mensajes.
4. Ejemplos/plantillas para no arrancar en blanco.

## Historias de usuario
1. Como usuario nuevo, quiero saber que cargar primero para entender la app sin friccion.
2. Como usuario nuevo, quiero ver ejemplos claros para completar mi primer movimiento rapido.
3. Como usuario nuevo, quiero entender como impacta cada movimiento en Balance y Tarjeta por pagar.

## Tareas de implementacion

### 1) Flujo guiado de primer uso
1. Detectar usuario nuevo con regla: `allExpenses.length === 0`.
2. Mostrar welcome modal/panel con 3 pasos:
   - Paso 1: cargar ingreso inicial.
   - Paso 2: cargar primer gasto.
   - Paso 3: explicar tarjeta por pagar y pago de tarjeta.
3. Incluir CTA por paso (abrir formulario con tipo preseleccionado).
4. Permitir cerrar/saltar y guardar estado para no reaparecer en cada carga.

### 2) Empty states utiles
1. Home sin movimientos:
   - tarjeta visual con "Que hacer ahora" + botones de accion.
2. Tabla sin datos:
   - mensaje corto + CTA "Cargar primer movimiento".
3. Tarjeta por pagar sin pendiente:
   - estado vacio explicando cuando se llena.
4. Calendario sin items en el mes:
   - sugerencia de registrar ingreso/gasto inicial.

### 3) Copy simple y consistente
1. Unificar etiquetas:
   - mantener `Medio de pago` y `Forma de pago` segun la vista.
2. Revisar microcopy del formulario:
   - reducir texto tecnico.
   - mantener explicacion de impacto por metodo.
3. Revisar mensajes de exito/error:
   - texto breve, accionable y sin ambiguedad.
4. Validar consistencia entre home, tabla, settings y landing FAQ.

### 4) Ejemplos y plantillas
1. Placeholder con ejemplos locales:
   - "Ej: Supermercado", "Ej: Alquiler", "Ej: Sueldo".
2. Plantilla rapida opcional:
   - boton "Cargar 3 ejemplos" en modo demo/nuevo usuario.
3. Tooltips de ayuda:
   - cuotas/recurrentes y tarjeta por pagar.

### 5) QA y cierre
1. QA responsive:
   - 360, 390, 414, 430, 768, >=1280.
2. Smoke funcional:
   - alta ingreso, gasto debito/transferencia, gasto credito, pago propio, pago tercero.
3. Confirmar que no se rompen filtros ni tabla ni calendario.
4. Actualizar docs:
   - `docs/product-redesign-plan.md` (estado de onboarding).
   - FAQ de landing con flujo de primer uso.

## Criterios de aceptacion (Definition of Done)
1. Usuario nuevo entiende en menos de 2 minutos que cargar primero.
2. La app no muestra pantallas vacias sin contexto ni accion sugerida.
3. El onboarding no molesta a usuarios existentes.
4. El copy es consistente en todas las vistas clave.
5. Se ejecuta QA manual completo y queda documentado.
6. Cambios listos para merge a `dev`.

## Entregables
1. UI onboarding (welcome + empty states + ayudas contextuales).
2. Copy unificado y validado.
3. Checklist de QA ejecutado.
4. Documentacion actualizada.
