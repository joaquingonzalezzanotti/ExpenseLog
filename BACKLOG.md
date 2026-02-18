# Backlog

## Plan Por Sprints (Dev -> Main)

- Estado actual: Sprint 1 completado, Sprint 2 en curso (dev).

### Sprint 1 (en curso) - Higiene UX mobile + control de alcance
- Objetivo: mejorar legibilidad/usabilidad sin tocar logica de negocio de gastos.
- Alcance:
- `P0 - Desactivar import/export CSV temporalmente`
- `P1 - Filtros del panel mobile mas claros y equilibrados`
- `P1 - Header mobile mas compacto (logo, notificacion y acciones)`
- `P2 - Mejor contraste visual en tarjeta "Saldo inicial"`
- `P2 - Tabla principal: alineacion y jerarquia visual` (ajuste visual minimo)
- Entregable: UI mas clara en mobile/oscuro + CSV desactivado en UI y backend.
- Riesgo: bajo.
- Pase a main: smoke test manual desktop/mobile + `go test ./...`.

### Sprint 2 - Home y tabla mobile orientadas a operacion diaria
- Objetivo: reducir friccion de carga y lectura en mobile.
- Alcance:
- `P1 - Orden de bloques en home: movimientos antes de grafico`
- `P0 - Table mobile: filtros y acciones sin overflow`
- `P0 - Table mobile: paginacion simple y formulario colapsado`
- `P1 - Prioridad de columnas en table mobile`
- Entregable: tabla usable en pantallas chicas y home mas operativa.
- Riesgo: medio (cambios de layout + interacciones).
- Pase a main: validacion manual en 320/360/390 + regression de alta/edicion/borrado.

### Sprint 3 - Recurrentes y semantica de proyeccion
- Objetivo: eliminar ambiguedades entre saldo real y proyeccion.
- Alcance:
- `P0 - Definir semantica de panel en mes futuro`
- `P1 - Reglas de negocio documentadas para recurrentes`
- `P1 - Estados visuales de notificacion/alerta consistentes`
- `P1 - Microcopy orientado a decision`
- Entregable: estados claros (real/proyectado) y mensajes accionables.
- Riesgo: medio-alto (impacta interpretacion funcional).
- Pase a main: aprobacion de casos reales antes/despues en capturas y checklist.

### Sprint 4 - Calidad y deuda tecnica de alertas/fechas
- Objetivo: blindar comportamiento con tests y consolidar reglas.
- Alcance:
- `P0 - Cobertura de casos criticos de liquidez`
- `P1 - Tests de integracion UI para visibilidad de alertas`
- `P1 - Extraer reglas temporales a configuracion central`
- `P2 - Hardening de manejo de fechas (timezone-safe)`
- `P2 - Limpieza de estilos y tokens de tema`
- Entregable: menor regresion y codigo mas mantenible.
- Riesgo: medio.
- Pase a main: CI verde + matriz de casos de liquidez validada.

## Producto

### P0 - Alertas de recurrentes confiables end-to-end
- Problema: En algunos escenarios el backend calcula alerta, pero el usuario no siempre la ve en UI (dismiss, filtros o estado temporal).
- Resultado esperado: Las alertas de 7 dias y 4 dias aparecen cuando corresponde y no desaparecen erroneamente.
- Criterio de aceptacion (DoD):
- Si falta entre 7 y 5 dias: notificacion informativa visible.
- Si falta entre 4 y 1 dias y saldo proyectado < 0: alerta critica visible.
- Si el usuario borra una notificacion, reaparece al entrar en ventana de 24h.
- En fecha de vencimiento o vencido: solo informativa.
- Impacto: Alto (confianza del usuario en la app).
- Esfuerzo: M.

### P0 - Definir semantica de panel en mes futuro
- Problema: El panel de un mes futuro puede mostrar valores que parecen "saldo real" cuando en realidad son proyeccion.
- Resultado esperado: La app distingue claramente entre estado real a hoy y proyeccion futura.
- Criterio de aceptacion (DoD):
- Para mes actual: metricas con movimientos hasta ahora.
- Para mes futuro: etiqueta explicita "Proyeccion".
- "Ultimos movimientos" cambia a "Proximos movimientos" en vistas futuras.
- Impacto: Alto (evita decisiones erroneas).
- Esfuerzo: M.

### P1 - Reglas de negocio documentadas para recurrentes
- Problema: Hay ambiguedad sobre cuando una recurrente "cuenta" en saldo y alertas.
- Resultado esperado: Reglas de negocio claras y estables para equipo y usuarios.
- Criterio de aceptacion (DoD):
- Documento corto en repo con ejemplos de casos reales.
- Mapeo explicito de reglas: 7d, 4d, 24h, vencido.
- Referencia cruzada con tests automatizados.
- Impacto: Medio-alto.
- Esfuerzo: S.

## UX/UI

### P1 - Estados visuales de notificacion/alerta consistentes
- Problema: Puede no quedar claro que es "info" y que es "critico".
- Resultado esperado: Jerarquia visual clara y consistente entre estados.
- Criterio de aceptacion (DoD):
- Badge, color e icono diferenciados para `info` y `critica`.
- Mensajes en espanol consistentes y sin ambiguedades.
- Contraste AA en modo claro y oscuro en componentes de alerta.
- Impacto: Medio.
- Esfuerzo: S.

### P1 - Microcopy orientado a decision
- Problema: Frases como "riesgo en X dias" no siempre explican accion ni contexto.
- Resultado esperado: Mensajes accionables ("Saldo proyectado luego del egreso", "Faltante estimado").
- Criterio de aceptacion (DoD):
- Revisar copies de panel/alerta/cashflow.
- Alinear terminos: notificacion, alerta, vencimiento, proyeccion.
- Validacion rapida con 3 escenarios manuales.
- Impacto: Medio.
- Esfuerzo: S.

### P1 - Header mobile mas compacto (logo, notificacion y acciones)
- Problema: En mobile, logo + botones + notificacion ocupan mucho alto y desplazan contenido clave.
- Resultado esperado: Header compacto y util sin perder accesos.
- Criterio de aceptacion (DoD):
- Reducir altura total del header en mobile.
- Ajustar tamano/espaciado de logo y botones.
- El boton de notificacion no debe tapar ni romper layout.
- Impacto: Alto (usabilidad mobile).
- Esfuerzo: M.

### P1 - Formulario de gasto con foco en accion principal
- Problema: El boton de agregar gasto no destaca lo suficiente y el toggle de registrar ingreso confunde en alta.
- Resultado esperado: CTA principal claro y flujo mas simple para alta.
- Criterio de aceptacion (DoD):
- Boton "Agregar gasto" con mayor jerarquia visual.
- En alta: ocultar/simplificar toggle de tipo si no aporta.
- En edicion: mantener control para corregir rapidamente el tipo.
- Impacto: Alto.
- Esfuerzo: M.

### P1 - Tabla de recurrentes responsive y boxed
- Problema: La tabla de recurrentes en settings ocupa demasiado en mobile y no mantiene patron visual de tabla principal.
- Resultado esperado: Tabla de recurrentes contenida y legible en mobile.
- Criterio de aceptacion (DoD):
- Contenedor boxed con scroll horizontal controlado o layout adaptado.
- Tipografia/espaciado consistentes con `table.html`.
- Sin overflow horizontal fuera del viewport.
- Impacto: Medio-alto.
- Esfuerzo: M.

### P2 - Revisar quick actions (evitar doble "Gasto")
- Problema: Hay acciones redundantes ("Gasto" y "Gasto con Tarjeta") que pueden confundir.
- Resultado esperado: Set de acciones rapido, sin duplicidad conceptual.
- Criterio de aceptacion (DoD):
- Definir nomenclatura final de botones.
- Mantener acceso rapido a gasto con tarjeta.
- Validar que no se rompa el flujo de carga.
- Impacto: Medio.
- Esfuerzo: S.

### P2 - Tabla principal: alineacion y jerarquia visual
- Problema: Alineaciones no siempre ayudan lectura (centrado/justificado inconsistente).
- Resultado esperado: Lectura de filas mas rapida y consistente.
- Criterio de aceptacion (DoD):
- Definir alineacion por tipo de columna (texto, monto, fecha, acciones).
- Mantener contraste y pesos tipograficos claros.
- Verificado en desktop y mobile.
- Impacto: Medio.
- Esfuerzo: S.

### P2 - Mejor contraste visual en tarjeta "Saldo inicial"
- Problema: El bloque de saldo inicial puede perderse contra el fondo.
- Resultado esperado: Bloque distinguible sin romper la paleta.
- Criterio de aceptacion (DoD):
- Fondo claro diferenciado para saldo inicial.
- Contraste legible en claro y oscuro.
- Coherencia con tarjetas de ingresos/gastos/balance.
- Impacto: Medio.
- Esfuerzo: S.

### P1 - Filtros del panel mobile mas claros y equilibrados
- Problema: El texto de "moneda base..." agrega ruido y en mobile los botones de filtros no tienen equilibrio visual.
- Resultado esperado: Barra de filtros limpia y usable en pantallas chicas.
- Criterio de aceptacion (DoD):
- Quitar/relocalizar el texto de moneda base del panel principal.
- En mobile, "Limpiar" y "Ver grafico" con mismo ancho y alineacion.
- Sin saltos de linea que rompan la fila de filtros.
- Impacto: Medio-alto.
- Esfuerzo: S.

### P1 - Orden de bloques en home: movimientos antes de grafico
- Problema: El contenido relevante de movimientos recientes compite con el grafico.
- Resultado esperado: Priorizar informacion operativa antes de analitica.
- Criterio de aceptacion (DoD):
- Mostrar bloque de movimientos antes del grafico en home.
- Mantener comportamiento actual de expandir/ocultar grafico.
- Verificado en desktop y mobile.
- Impacto: Medio.
- Esfuerzo: S.

### P0 - Table mobile: filtros y acciones sin overflow
- Problema: En `table.html` mobile, filtros y boton de limpiar pueden salir de pantalla.
- Resultado esperado: Filtros 100% contenidos en viewport.
- Criterio de aceptacion (DoD):
- Filtros y boton de limpiar sin overflow horizontal.
- Inputs/selects con ancho responsive real.
- Prueba manual en anchos 320, 360 y 390.
- Impacto: Alto.
- Esfuerzo: M.

### P1 - Localizacion completa de labels de intervalos
- Problema: En recurrentes se muestran valores en ingles (daily, weekly, etc.).
- Resultado esperado: Todo label de usuario en espanol.
- Criterio de aceptacion (DoD):
- Mostrar Diario, Semanal, Mensual, Anual en UI.
- Mantener compatibilidad con valores internos actuales.
- Sin regressions en alta/edicion/listado.
- Impacto: Medio.
- Esfuerzo: S.

### P0 - Table mobile: paginacion simple y formulario colapsado
- Problema: En mobile la tabla larga y el formulario abierto por defecto degradan experiencia.
- Resultado esperado: Pantalla enfocada en lectura con expansion bajo demanda.
- Criterio de aceptacion (DoD):
- Mostrar 10 gastos por defecto en mobile y boton "Mostrar todos".
- Formulario "Agregar gasto" colapsado por defecto y desplegable con boton.
- Mantener flujo de alta sin pasos extra innecesarios.
- Impacto: Alto.
- Esfuerzo: M.

### P1 - Prioridad de columnas en table mobile
- Problema: En mobile no se priorizan suficientemente las columnas clave.
- Resultado esperado: Primera vista con Nombre, Valor y Balance.
- Criterio de aceptacion (DoD):
- Orden inicial mobile: Nombre, Valor, Balance.
- Categoria/medio de pago/otras columnas como secundarias (expandibles o debajo).
- Sin perdida de informacion funcional.
- Impacto: Medio-alto.
- Esfuerzo: M.

### P0 - Desactivar import/export CSV temporalmente
- Problema: Se desea remover por ahora importacion/exportacion CSV de toda la app.
- Resultado esperado: Funcionalidad deshabilitada en desktop y mobile.
- Criterio de aceptacion (DoD):
- Ocultar o desactivar controles de import/export en UI.
- Endpoint bloqueado o protegido para evitar uso accidental.
- Sin accesos residuales visibles al usuario final.
- Impacto: Alto (control de alcance y soporte).
- Esfuerzo: S.

### P1 - Reducir ruido de fetch/logs de categorias en cliente
- Problema: Se observa fetch repetido y logs de categorias en consola al navegar modulos.
- Resultado esperado: Menos llamadas redundantes y consola limpia en produccion.
- Criterio de aceptacion (DoD):
- Revisar ciclos de inicializacion y cache de categorias.
- Remover logs de debug en cliente para produccion.
- Verificar que categorias sigan sincronizadas.
- Impacto: Medio.
- Esfuerzo: M.

### P1 - Settings mobile: listas y formularios colapsables
- Problema: En mobile settings muestra demasiados bloques abiertos (categorias y recurrentes) de entrada.
- Resultado esperado: Pantalla mas ordenada por secciones desplegables.
- Criterio de aceptacion (DoD):
- Boton "Ver categorias/Ocultar categorias".
- Formulario "Agregar transaccion recurrente" colapsado por defecto con boton para desplegar.
- Estado de despliegue estable durante la sesion.
- Impacto: Medio-alto.
- Esfuerzo: M.

## Calidad/Tests

### P0 - Cobertura de casos criticos de liquidez
- Problema: La regresion de alertas puede reaparecer sin detectarse temprano.
- Resultado esperado: Suite de tests que cubra escenarios reales y bordes.
- Criterio de aceptacion (DoD):
- Tests backend para 8 casos definidos (ventana 7d/4d, vencido, acumulacion, timezone).
- Tests pasan en `go test ./...`.
- Casos documentados en README tecnico o comentario en test.
- Impacto: Alto.
- Esfuerzo: M.

### P1 - Tests de integracion UI para visibilidad de alertas
- Problema: El calculo puede estar bien y la UI fallar igual.
- Resultado esperado: Validacion de render y dismiss/reaparicion.
- Criterio de aceptacion (DoD):
- Test de render con payload simulado de `/alerts/liquidity`.
- Test de `Borrar` + reaparicion en 24h.
- Verificacion de prioridad visual critica sobre informativa.
- Impacto: Alto.
- Esfuerzo: M.

## Deuda tecnica

### P1 - Extraer reglas temporales a configuracion central
- Problema: Ventanas de 7/4/1 dias estan dispersas en codigo.
- Resultado esperado: Parametros de negocio centralizados y reutilizables.
- Criterio de aceptacion (DoD):
- Constantes unificadas backend/frontend.
- Evitar numeros magicos en logica de alertas.
- Tests siguen pasando sin cambios de comportamiento.
- Impacto: Medio.
- Esfuerzo: S.

### P2 - Hardening de manejo de fechas (timezone-safe)
- Problema: Diferencias UTC/local pueden desplazar fechas de recurrentes.
- Resultado esperado: Estrategia unica de fecha-calendario para recurrentes.
- Criterio de aceptacion (DoD):
- Guardado y lectura date-only consistentes.
- Sin desfase de dia en alta/edicion/listado/alertas.
- Caso borde timezone cubierto por test.
- Impacto: Medio-alto.
- Esfuerzo: M.

### P2 - Limpieza de estilos y tokens de tema
- Problema: Hay estilos de alerta y panel que crecieron por iteraciones.
- Resultado esperado: CSS mas mantenible sin cambiar comportamiento.
- Criterio de aceptacion (DoD):
- Consolidar tokens y variantes duplicadas.
- Mantener paridad visual claro/oscuro.
- Sin regresiones visibles en index/table/settings.
- Impacto: Medio.
- Esfuerzo: M.
