# Plan de trabajo rama `metricas`

Fecha: 2026-03-24  
Rama: `metricas`

## Objetivo
Implementar metricas internas para el equipo de desarrollo (no visibles para usuarios finales) para medir salud del sistema, calidad de codigo y velocidad de entrega.

## Alcance
- Backend y operacion de la app.
- Instrumentacion tecnica (logs, contadores, tiempos, errores).
- Reportes y/o panel interno para el equipo.
- Sin cambios de producto orientados a usuarios finales.

## Entregables
1. Documento de definicion de metricas (fuente, formula, frecuencia, owner).
2. Instrumentacion minima en servidor (request metrics, errores, latencia).
3. Indicadores DORA basicos calculables desde git/deploy logs.
4. Script o endpoint interno para extraer metricas agregadas.
5. Dashboard tecnico interno (o reporte markdown/CSV inicial).
6. Tests para validar calculo de metricas clave.

## Metricas objetivo (MVP)
1. Flujo de entrega (DORA)
- Deployment Frequency
- Lead Time for Changes
- Change Failure Rate
- Mean Time to Recovery (MTTR)

2. Fiabilidad operativa
- Tasa de errores por endpoint
- Latencia p50/p95 por endpoint critico
- Disponibilidad de rutas principales

3. Calidad de codigo
- Tasa de tests pasando/fallando en CI
- Cobertura (si aplica)
- Defectos detectados post-deploy

4. Salud de producto tecnico
- Errores de validacion frecuentes
- Reintentos/idempotencia (cuando aplique)
- Tiempo de respuesta en operaciones clave (crear gasto, listar, reportes)

## Plan tecnico
1. Definicion de contrato de metricas
- Definir cada metrica con nombre canonico, formula, ventana temporal y fuente.
- Identificar owners por metrica.

2. Instrumentacion backend
- Agregar contadores y timers en middleware/handlers.
- Estandarizar logging estructurado para trazabilidad.
- Etiquetar errores por tipo y endpoint.

3. Recoleccion y agregacion
- Crear componente para agregar metricas por periodo (dia/semana/mes).
- Exponer lectura por endpoint interno y/o generar archivo de reporte.

4. Integracion con flujo de entrega
- Usar historial git y eventos de deploy para DORA.
- Automatizar un resumen semanal (script o tarea manual guiada).

5. Validacion
- Tests unitarios para formulas de agregacion.
- Verificacion cruzada con logs reales.
- Control de costo de instrumentacion (no degradar performance).

## Checklist operativo
- [ ] Definir diccionario de metricas internas.
- [ ] Mapear fuentes de datos (logs, git, deploys, pruebas).
- [ ] Instrumentar contadores de requests/errores/latencia.
- [ ] Implementar agregaciones por periodo.
- [ ] Crear salida consumible (endpoint interno o reporte).
- [ ] Agregar tests de calculo.
- [ ] Documentar uso e interpretacion para el equipo.

## Riesgos y mitigacion
- Riesgo: metricas inconsistentes por falta de definiciones.
  - Mitigacion: diccionario de metricas versionado en `docs/`.
- Riesgo: sobreinstrumentacion y ruido.
  - Mitigacion: empezar por MVP y ampliar por iteraciones.
- Riesgo: impacto en rendimiento.
  - Mitigacion: medicion de overhead y sampling si es necesario.

## Definicion de hecho (DoD)
- Set minimo de metricas internas implementado y documentado.
- Metodo reproducible para calcular DORA y metricas operativas.
- Tests de agregacion pasando.
- Reporte tecnico inicial disponible para el equipo.
