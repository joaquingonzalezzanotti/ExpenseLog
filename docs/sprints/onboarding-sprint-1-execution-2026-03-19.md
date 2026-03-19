# Sprint 1 - Onboarding Hardening (Ejecucion)

Fecha de inicio: 2026-03-19  
Rama de trabajo: `onboarding`  
Base integrada: `dev` mergeado en `onboarding` (fast-forward)

## 1) Objetivo del sprint
Cerrar onboarding para decision de merge a `dev` con QA completo, copy consistente y cero regresiones criticas.

## 2) Alcance comprometido
1. Verificar flujo de primer usuario en Home.
2. Verificar empty states en Home, Tabla y Calendario.
3. Ajustar copy de onboarding para consistencia.
4. Ejecutar QA responsive en breakpoints definidos.
5. Dejar checklist de salida completo para PR `onboarding -> dev`.

## 3) Fuera de alcance
1. Apple Wallet E2E.
2. Modulo de Ahorros.
3. Export Premium.
4. Refactors grandes fuera de onboarding.

## 4) Checklist de ejecucion

## A) Flujo onboarding Home
- [ ] Usuario nuevo (`allExpenses.length === 0`) ve panel de 3 pasos.
- [ ] CTA `Cargar ingreso` abre formulario preconfigurado.
- [ ] CTA `Cargar gasto` abre formulario preconfigurado.
- [ ] CTA `Cargar gasto con tarjeta` abre formulario preconfigurado.
- [ ] `Saltar por ahora` persiste estado y no reaparece en reload.

## B) Empty states
- [ ] Home vacio muestra guia accionable.
- [ ] Tabla vacia muestra estado contextual.
- [ ] Calendario vacio muestra estado contextual.
- [ ] Tarjeta por pagar sin pendiente muestra ayuda contextual.

## C) Copy y consistencia
- [ ] Terminologia consistente en CTA y mensajes.
- [ ] Sin mensajes ambiguos o tecnicos para usuario nuevo.
- [ ] Coherencia Home <-> Tabla <-> Calendario.

## D) QA responsive
- [ ] 360px
- [ ] 390px
- [ ] 414px
- [ ] 430px
- [ ] 768px
- [ ] >=1280px

## E) Smoke funcional
- [ ] Alta ingreso.
- [ ] Alta gasto transferencia/debito.
- [ ] Alta gasto tarjeta.
- [ ] Pago de tarjeta propio.
- [ ] Pago de tarjeta por tercero.
- [ ] Filtros Home/Tabla/Calendario no se rompen.

## 5) Criterio de salida del sprint
1. Checklist A-E completo.
2. Sin bugs P0/P1 abiertos de onboarding.
3. Documentacion actualizada.
4. PR `onboarding -> dev` listo para revision.

## 6) Log diario (plantilla)

## Daily YYYY-MM-DD
1. Hecho:
2. En curso:
3. Bloqueos:
4. Decision del dia:

