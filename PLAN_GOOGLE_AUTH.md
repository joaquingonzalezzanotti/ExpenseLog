# Plan de implementación por sprints: Login/Sign up con Google

## Sprint 0 - Diseño y alineación (1-2 días)
- Definir requerimientos de negocio:
  - ¿Permitir login solo con Google o link con cuentas existentes por email?
  - ¿Crear cuentas nuevas automáticamente si el email no existe?
  - ¿Soportar unlink/revoke?
- Definir políticas de seguridad:
  - Expiración de sesión, rotación, manejo de `state` y `nonce`.
  - Manejo de cuentas suspendidas/bloqueadas.
- Definir variables de entorno:
  - `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URL`.
- Diseño de esquema (aprobación):
  - Tabla `oauth_identities` (provider, provider_user_id, email, user_id).
  - `users.password_hash` nullable para cuentas OAuth.

## Sprint 1 - Backend OAuth base (2-4 días)
- Implementar endpoints:
  - `GET /auth/google` (iniciar OAuth, crear `state` + `nonce`).
  - `GET /auth/google/callback` (intercambio de código, validación de tokens).
- Validación de tokens:
  - Verificar `aud`, `iss`, `exp`, `email_verified`.
  - Uso de `nonce` (si aplica) y validación de `state` anti-CSRF.
- Persistencia:
  - Buscar o crear usuario + identidad OAuth.
  - Crear sesión reutilizando `createSession`.
- Logs y métricas básicas.

## Sprint 2 - UI/UX (1-2 días)
- Agregar botón “Continuar con Google” en overlay de auth.
- Ajustar mensajes de error en overlay para fallos OAuth.
- Manejo de estado post-callback (redirección limpia al dashboard).

## Sprint 3 - Hardening y operaciones (1-2 días)
- Casos borde:
  - Conflicto de emails (cuenta local vs OAuth).
  - Usuario OAuth sin email verificable.
- Agregar tests (unitarios de handlers + validación de tokens).
- Documentación de deploy y pasos en README.

## Sprint 4 - Extras opcionales (1-2 días)
- Link/unlink de cuentas desde settings.
- Soporte multi-provider (Apple/Microsoft) reutilizando la misma tabla.

---

## Entregables acordados para este PR
- Cambios de esquema iniciales (tabla `oauth_identities` y `password_hash` nullable).
- Documento de plan por sprints (este archivo).
