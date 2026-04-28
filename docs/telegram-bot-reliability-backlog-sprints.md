# Telegram + WhatsApp Bot Reliability Backlog & Sprints

## Context
This backlog tracks the architectural redesign of bot reliability across Telegram and WhatsApp, focusing on:
- `income/expense` correctness in transfers,
- user-specific identity modeling (without hardcoded names, channel-agnostic),
- safer bot decisions and lower ambiguity across channels,
- robust ingestion for `media + caption` and multi-transaction text batches,
- conversation layer quality without mixing tone with accounting decisions,
- auditable media retention with controlled cost.

Branch baseline: `dev`  
Start date: `2026-04-28`

## Delivery Strategy
Rollout order:
1. `dev` (full implementation with feature flags off by default)
2. canary/staging validation
3. `prod` gradual rollout (`5% -> 25% -> 50% -> 100%`)

Release guardrails:
- additive migrations only,
- kill switches per feature,
- no destructive schema changes in rollout phases.

## Prioritized Backlog

### P0 (Critical reliability + architecture)
1. Introduce a channel-agnostic canonical parser contract (`ParseCandidate`, `ParseEvidence`, `ParseDecision`).
2. Centralize a unified decision engine for Telegram + WhatsApp (identity-aware, ambiguity-aware).
3. Generalize identity storage model to channel-agnostic records (`user identity aliases + owned account fingerprints + optional channel_scope`).
4. Keep channel adapters (Telegram/WhatsApp) isolated from accounting core.
5. Add Telegram event grouping for `image/pdf + caption` in a single atomic intake operation.
6. Add multi-transaction segmentation for text batches (one message, multiple movements).
7. Disable unsafe auto-confirmation on ambiguous decisions.

### P1 (Observability + UX + operational safety)
1. Add Premium+Linked gating UX for identity settings (cross-channel visibility rules).
2. Add conversation layer (persona/tone templates) above the unified core.
3. Add edit/undo/confirm conversational UX consistently across channels.
4. Add media-audit object model + purge policy.
5. Add decision telemetry (`confidence`, `ambiguity_reason`, `evidence`, `correction_loop`).

### P2 (Optimization and scale)
1. Minimize clarification prompts toward near-zero through feedback learning.
2. Latency tuning and cache strategy.
3. Precision monitoring dashboards and regression dataset automation.

## Sprint Plan

## Sprint 1 - Foundation Contracts & Data Model
Goal: Establish stable data/API contracts to support identity-aware parsing without breaking current production flow.

Scope:
1. Storage schema additions (identity aliases + account fingerprints).
2. Storage interface + repository methods.
3. Authenticated API endpoint for Telegram identity (`GET/PUT`).
4. Bot internal API endpoint for identity retrieval.
5. Feature flag gate (`TELEGRAM_IDENTITY_V2_ENABLED`).
6. Route wiring.

Status: **Completed (2026-04-28)**

Tasks:
- [x] Add `telegram_identity_aliases` table (additive migration style in `createTables`).
- [x] Add `telegram_owned_account_fingerprints` table (additive migration style in `createTables`).
- [x] Add indexes for both tables (uniqueness + user lookup paths).
- [x] Add FK enforcement for both tables.
- [x] Extend `storage.Storage` contract with identity methods.
- [x] Implement repository methods in `databaseStore`:
  - `GetTelegramIdentityAliases`
  - `ReplaceTelegramIdentityAliases`
  - `GetTelegramOwnedAccountFingerprints`
  - `ReplaceTelegramOwnedAccountFingerprints`
- [x] Add authenticated endpoint `GET/PUT /api/telegram/identity`.
- [x] Add internal endpoint `POST /api/bot/telegram/identity`.
- [x] Add feature gate `TELEGRAM_IDENTITY_V2_ENABLED` (default OFF).
- [x] Register new routes in `cmd/expenselog/main.go`.
- [x] Compile validation across packages (`go test ./... -run TestDoesNotExist`).
- [x] Add/extend tests for new identity endpoints and storage methods.
- [x] Add Settings UI section (Premium + linked only) wired to new endpoint.

Implemented files:
- `internal/storage/storage.go`
- `internal/storage/databaseStore.go`
- `internal/api/telegram_identity.go`
- `cmd/expenselog/main.go`
- `internal/api/telegram_identity_test.go`
- `internal/web/templates/settings.html`
- `internal/web/templates/style.css`

## Sprint 2 - Core Unification (Telegram + WhatsApp)
Goal: Build one shared accounting brain with channel adapters.

Planned scope:
1. Define canonical parse contract:
   - `ParseCandidate` (amount/currency/flow/date/counterparty/reference/source).
   - `ParseEvidence` (raw text, OCR spans, sender metadata, account hints).
   - `ParseDecision` (final flow/amount/category/source + confidence + reasons).
2. Add unified decision engine with identity-aware transfer classification.
3. Add channel adapters:
   - Telegram adapter (external parser payload -> canonical contract).
   - WhatsApp adapter (text/media parse result -> canonical contract).
4. Add ambiguity policy (no auto-confirm on low-confidence/mixed evidence).
5. Add semantic validators before persistence.

Status: **In progress (2026-04-28)**

Tasks:
- [x] Add `internal/botcore` canonical contract (`ParseCandidate`, `ParseEvidence`, `IdentityProfile`, `Decision`).
- [x] Add unified decision engine with confidence/reasons/ambiguity output.
- [x] Add channel adapters in API layer (`Telegram payload -> ParseCandidate`, `WhatsApp parsed -> ParseCandidate`).
- [x] Add feature flag `UNIFIED_DECISION_ENGINE_ENABLED` (default OFF).
- [x] Integrate unified decision path in:
  - `CreateBotExpense` (Telegram).
  - `saveWhatsAppParsedExpense` (WhatsApp).
- [x] Keep legacy paths as fallback while flag is OFF.
- [x] Add persistent homogeneous decision telemetry table and writer (foundation: insert-only events on unified path).
- [ ] Add telemetry read APIs/dashboards and correction-loop joins.
- [x] Implement explicit clarification state machine foundation for ambiguous decisions (Telegram internal confirm endpoint + persistent pending decisions).
- [x] Extend clarification state machine foundation to WhatsApp conversational confirm loop (`ingreso`/`gasto` over pending decision).
- [ ] Extend WhatsApp clarification loop with interactive buttons and timeout reminders.
- [x] Enable grouped Telegram attachment intake foundation (`media + caption`) with buffered assembly + dedupe behind `TELEGRAM_ATTACHMENT_ATOMIC_INTAKE_ENABLED`.
- [ ] Extend Telegram atomic intake with media-group ordering and persistence-backed buffering.
- [x] Implement multi-transaction text segmentation service (initial integration in WhatsApp text flow).
- [x] Extend multi-transaction ingestion to Telegram adapter path (initial `source_meta.batch_items` support).
- [ ] Extend Telegram batch to raw-text segmentation parity with WhatsApp.

Implemented files in Sprint 2 (current increment):
- `internal/botcore/decision.go`
- `internal/botcore/decision_test.go`
- `internal/api/bot_unified_core.go`
- `internal/api/telegram_bot.go`
- `internal/api/whatsapp_kapso_webhook.go`
- `internal/api/telegram_atomic_ingest.go`
- `internal/api/telegram_atomic_ingest_test.go`
- `internal/botcore/segmentation.go`
- `internal/botcore/segmentation_test.go`
- `internal/storage/storage.go`
- `internal/storage/databaseStore.go`
- `internal/api/telegram_bot_confirm.go`
- `internal/api/telegram_bot_confirm_test.go`
- `cmd/expenselog/main.go`

## Sprint 3 - Ingestion Reliability & Batch Parsing
Goal: Handle real chat behavior correctly before decision.

Planned scope:
1. Telegram atomic intake for attachment:
   - merge `media + caption` into one ingestion envelope,
   - dedupe by message/update identifiers,
   - handle album/media-group ordering.
2. Multi-transaction batch parser for text:
   - segment one message into N movement candidates,
   - classify each candidate independently,
   - aggregate summary + per-item correction handles.
3. Time normalization policy:
   - explicit timezone strategy to avoid +3h drift,
   - confidence downgrade on missing/contradictory time evidence.
4. Unified OCR/AI fallback path with consistent confidence semantics.

Status: **Pending**

## Sprint 4 - Conversation Layer & Safe UX
Goal: Add personality without sacrificing financial precision.

Planned scope:
1. Conversation layer templates by intent/state (`success`, `ambiguity`, `needs-confirmation`, `error`, `retry`).
2. Style guardrails:
   - friendly framing allowed,
   - accounting payload always explicit and machine-consistent.
3. Channel-consistent action UI:
   - confirm/edit/undo links/buttons with same semantics.
4. Premium configuration for tone level and verbosity.

Status: **Pending**

## Sprint 5 - Media Audit, Rollout, and Operations
Goal: Operate safely in Railway and ship progressively.

Planned scope:
1. Media-audit retention model (selective retention + TTL + purge job).
2. Deployment checklist (bucket per env, alerts, hard limits).
3. Canary rollout and KPI validation (`precision`, `manual-correction-rate`, `ambiguity-rate`, `p95 latency`).

Status: **Pending**

## Operational Notes
1. Keep `TELEGRAM_IDENTITY_V2_ENABLED=false` by default in all environments until Sprint 1 validation is complete.
2. Never share storage buckets across `dev` and `prod`.
3. Use additive migrations and feature flags as rollback mechanism.
4. Keep `UNIFIED_DECISION_ENGINE_ENABLED=false` until Sprint 2 and Sprint 3 validation is complete.
5. Keep `TELEGRAM_ATTACHMENT_ATOMIC_INTAKE_ENABLED=false` until Telegram `media+caption` grouping tests pass.
6. Keep `BOT_CONVERSATION_LAYER_ENABLED=false` until response consistency tests pass.
