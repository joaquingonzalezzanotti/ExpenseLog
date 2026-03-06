import { Telegraf } from 'telegraf';
import { prisma } from '../db/client.js';
import { config, isAllowedTelegramUser } from '../config.js';
import { tempPathFor, safeDelete } from '../processing/file.js';
import { processReceipt } from '../processing/pipeline.js';
import { canUseAIParser, parseWithAIParser } from '../processing/ai_parser.js';
import { logger } from '../observability/logger.js';
import { draftSummary } from './format.js';
import { dedupeKeyboard, fixMenuKeyboard, mainDecisionKeyboard } from './keyboards.js';
import { ExpenseLogAdapter, ExpenseLogAPIError } from '../expenselog/client.js';
import { applyRules } from '../rules/engine.js';
import { draftResults, parseResults, receiptsReceived, stageLatency } from '../observability/metrics.js';
import { writeFile } from 'node:fs/promises';
const expenselogClient = new ExpenseLogAdapter();
const pendingFix = new Map();
const linkStatusCache = new Map();
const LINK_STATUS_CACHE_TTL_MS = 30_000;
const multiUserLinkingEnabled = Boolean(String(config.expenselogBotInternalSecret || '').trim());
const toReceiptParseResult = (value) => value;
const fieldPromptByKey = {
    source_app: { label: 'metodo de pago', hint: 'Escribi: "transferencia", "efectivo" o "tarjeta credito".' },
    type: { label: 'tipo', hint: 'Escribi "gasto" o "ingreso".' },
    amount: { label: 'monto', hint: 'Ejemplo: 1600 o 1600,50' },
    currency: { label: 'moneda', hint: 'Ejemplo: ARS' },
    datetime_iso: { label: 'fecha y hora', hint: 'Ejemplo: 24/02/2026 20:55' },
    counterparty: { label: 'contraparte', hint: 'Ejemplo: SUPER SALTO 273' },
    reference: { label: 'referencia', hint: 'Ejemplo: HX7UOTFO' },
    motive: { label: 'motivo', hint: 'Ejemplo: supermercado' },
    account_from: { label: 'cuenta origen', hint: '' },
    account_to: { label: 'cuenta destino', hint: '' },
    cbu_cvu_last4: { label: 'ultimos 4 CBU/CVU', hint: '' },
    confidence: { label: 'confianza', hint: '' },
    raw_text_excerpt: { label: 'texto', hint: '' },
    telegram_meta: { label: 'metadatos', hint: '' },
    rule_output: { label: 'regla', hint: '' }
};
const parseTypeInput = (raw) => {
    const normalized = raw.trim().toLowerCase();
    if (['gasto', 'egreso', 'expense'].includes(normalized))
        return 'expense';
    if (['ingreso', 'income'].includes(normalized))
        return 'income';
    return undefined;
};
const normalizeForMethodMatch = (raw) => String(raw || '')
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .trim()
    .toUpperCase();
const normalizePaymentMethod = (raw) => {
    const normalized = normalizeForMethodMatch(raw);
    if (!normalized)
        return 'CA';
    if (normalized === 'EFECTIVO' || normalized.includes('CASH'))
        return 'EFECTIVO';
    if (normalized.includes('DEBITO') ||
        normalized.includes('DEBIT') ||
        normalized.includes('TRANSFER') ||
        normalized.includes('BANK') ||
        normalized.includes('BANCO') ||
        normalized.includes('WALLET') ||
        normalized.includes('MODO') ||
        normalized.includes('GALICIA')) {
        return 'CA';
    }
    if (normalized === 'TARJETA' ||
        normalized.includes('CREDITO') ||
        normalized.includes('CREDIT') ||
        normalized.includes('MASTERCARD') ||
        normalized.includes('AMEX') ||
        normalized.includes('VISA')) {
        return 'TARJETA';
    }
    return 'CA';
};
const parsePaymentMethodInput = (raw) => {
    const normalized = normalizeForMethodMatch(raw).toLowerCase();
    if (!normalized)
        return undefined;
    if (normalized.includes('efectivo') || normalized.includes('cash')) {
        return 'EFECTIVO';
    }
    if (normalized.includes('transfer') || normalized.includes('debito') || normalized.includes('debit') || normalized.includes('banco') || normalized.includes('bank') || normalized.includes('modo')) {
        return 'CA';
    }
    if (normalized.includes('tarjeta') || normalized.includes('credito') || normalized.includes('credit') || normalized.includes('visa') || normalized.includes('master') || normalized.includes('amex')) {
        return 'TARJETA';
    }
    return undefined;
};
const getMissingRequiredLabels = (r) => {
    const missing = [];
    if (!r.type)
        missing.push('tipo');
    if (!(typeof r.amount === 'number' && Number.isFinite(r.amount) && r.amount !== 0))
        missing.push('monto');
    if (!r.datetime_iso)
        missing.push('fecha y hora');
    if (!r.counterparty)
        missing.push('contraparte');
    return missing;
};
const ensurePrivateAllowed = async (ctx) => {
    const userId = ctx.from?.id;
    if (!userId || !isAllowedTelegramUser(userId)) {
        await ctx.reply('No tengo permiso para usar esta cuenta de Telegram.');
        return false;
    }
    if (ctx.chat?.type !== 'private') {
        await ctx.reply('Este bot funciona solo en chat privado (1 a 1).');
        return false;
    }
    return true;
};
const resolveLinkGateResult = async (telegramUserId, forceRefresh = false) => {
    if (!multiUserLinkingEnabled) {
        return { ok: true, reason: 'ok' };
    }
    const cached = linkStatusCache.get(telegramUserId);
    const now = Date.now();
    if (!forceRefresh && cached && cached.expiresAt > now) {
        return cached.value;
    }
    try {
        const status = await expenselogClient.getTelegramLinkStatus(telegramUserId);
        const result = status.linked && status.premium
            ? { ok: true, reason: 'ok' }
            : { ok: false, reason: status.premium ? 'not_linked' : 'premium_required' };
        linkStatusCache.set(telegramUserId, { expiresAt: now + LINK_STATUS_CACHE_TTL_MS, value: result });
        return result;
    }
    catch (error) {
        if (error instanceof ExpenseLogAPIError) {
            if (error.code === 'not_linked' || error.status === 404) {
                const result = { ok: false, reason: 'not_linked' };
                linkStatusCache.set(telegramUserId, { expiresAt: now + LINK_STATUS_CACHE_TTL_MS, value: result });
                return result;
            }
            if (error.code === 'premium_required' || error.status === 403) {
                const result = { ok: false, reason: 'premium_required' };
                linkStatusCache.set(telegramUserId, { expiresAt: now + LINK_STATUS_CACHE_TTL_MS, value: result });
                return result;
            }
            if (error.status === 401) {
                const result = { ok: false, reason: 'unauthorized' };
                linkStatusCache.set(telegramUserId, { expiresAt: now + LINK_STATUS_CACHE_TTL_MS, value: result });
                return result;
            }
        }
        logger.warn('telegram_link_status_failed', { telegramUserId, error });
        const result = { ok: false, reason: 'unknown' };
        linkStatusCache.set(telegramUserId, { expiresAt: now + 10_000, value: result });
        return result;
    }
};
const requireLinkedPremium = async (ctx, forceRefresh = false) => {
    const userId = ctx.from?.id;
    if (!userId)
        return false;
    const gate = await resolveLinkGateResult(userId, forceRefresh);
    if (gate.ok)
        return true;
    if (gate.reason === 'not_linked') {
        await ctx.reply('Primero vincula tu cuenta.\n1) En ExpenseLog: Settings > Telegram Bot > Generar codigo.\n2) Aca: /vincular TU-CODIGO');
        return false;
    }
    if (gate.reason === 'premium_required') {
        await ctx.reply('Tu cuenta de ExpenseLog no tiene Premium activo. Activalo y volve a intentar.');
        return false;
    }
    if (gate.reason === 'unauthorized') {
        await ctx.reply('No pude validar tu sesion con ExpenseLog en este momento. Proba de nuevo en unos minutos.');
        return false;
    }
    await ctx.reply('No pude validar la vinculacion con ExpenseLog. Proba de nuevo.');
    return false;
};
const hasRequiredAmount = (amount) => (typeof amount === 'number' && Number.isFinite(amount) && amount !== 0);
const mustHaveRequired = (r) => Boolean(r.type &&
    hasRequiredAmount(r.amount) &&
    r.datetime_iso &&
    r.counterparty);
const normalizeCounterparty = (value) => (String(value || '').trim().toLowerCase().replace(/\s+/g, ' '));
const dedupeKey = (r) => (`${r.type || 'unknown'}|${hasRequiredAmount(r.amount) ? Math.abs(r.amount).toFixed(2) : '0'}|${normalizeCounterparty(r.counterparty)}`);
const isWithin24Hours = (aIso, bIso) => {
    const aMs = Date.parse(String(aIso || ''));
    const bMs = Date.parse(String(bIso || ''));
    if (!Number.isFinite(aMs) || !Number.isFinite(bMs))
        return false;
    const diffMs = Math.abs(aMs - bMs);
    return diffMs <= (24 * 60 * 60 * 1000);
};
const latestDraftByUser = async (telegramUserId) => (prisma.receiptDraft.findFirst({ where: { telegramUserId }, orderBy: { updatedAt: 'desc' } }));
const answerCallback = async (ctx) => {
    if (typeof ctx.answerCbQuery !== 'function')
        return;
    await ctx.answerCbQuery().catch(() => undefined);
};
const findFallbackDuplicateByTimeWindow = async (telegramUserId, parsed) => {
    const candidates = await prisma.receiptDraft.findMany({
        where: {
            telegramUserId,
            dedupeKey: dedupeKey(parsed),
            status: 'confirmed'
        },
        orderBy: { updatedAt: 'desc' },
        take: 25
    });
    for (const candidate of candidates) {
        const candidateResult = toReceiptParseResult(candidate.parseResultJson);
        if (isWithin24Hours(candidateResult.datetime_iso, parsed.datetime_iso)) {
            return candidate;
        }
    }
    return null;
};
const mapLinkErrorToMessage = (error) => {
    if (error.code === 'invalid_link_code')
        return 'El codigo no es valido. Revisalo e intenta de nuevo.';
    if (error.code === 'link_code_expired')
        return 'Ese codigo ya vencio. Genera uno nuevo desde ExpenseLog.';
    if (error.code === 'link_code_used')
        return 'Ese codigo ya fue usado. Genera uno nuevo.';
    if (error.code === 'premium_required')
        return 'Necesitas Premium activo para vincular Telegram.';
    if (error.code === 'already_linked')
        return 'Tu cuenta de ExpenseLog ya esta vinculada a otro Telegram.';
    if (error.code === 'telegram_already_linked')
        return 'Este Telegram ya esta vinculado a otra cuenta.';
    if (error.status === 401)
        return 'No pude autenticar el bot contra ExpenseLog.';
    return error.message || 'No pude completar la vinculacion en este momento.';
};
const normalizeLinkCode = (raw) => {
    const compact = String(raw || '')
        .toUpperCase()
        .replace(/[^A-Z0-9]/g, '');
    if (compact.length !== 8)
        return '';
    return `${compact.slice(0, 4)}-${compact.slice(4)}`;
};
const buildAIParserURLForDiag = () => {
    const base = String(config.aiParserBaseUrl || '').trim().replace(/\/+$/, '');
    const pathRaw = String(config.aiParserParsePath || '/api/parse').trim();
    const path = pathRaw.startsWith('/') ? pathRaw : `/${pathRaw}`;
    if (!base) {
        return '';
    }
    return `${base}${path}`;
};
const formatDiagError = (error) => {
    const parts = [];
    const name = String(error?.name || '').trim();
    const message = String(error?.message || '').trim();
    if (name || message) {
        parts.push(`${name || 'Error'}: ${message || 'sin mensaje'}`);
    }
    const cause = error?.cause;
    if (cause) {
        const causeName = String(cause?.name || '').trim();
        const causeMessage = String(cause?.message || '').trim();
        const causeCode = String(cause?.code || '').trim();
        const causeHost = String(cause?.hostname || '').trim();
        const causePort = String(cause?.port || '').trim();
        const details = [
            causeName ? `cause=${causeName}` : '',
            causeMessage ? `msg=${causeMessage}` : '',
            causeCode ? `code=${causeCode}` : '',
            causeHost ? `host=${causeHost}` : '',
            causePort ? `port=${causePort}` : ''
        ].filter(Boolean);
        if (details.length > 0) {
            parts.push(details.join(' '));
        }
    }
    return parts.join(' | ') || 'Error desconocido';
};
const fetchWithTimeout = async (url, init, timeoutMs) => {
    const controller = new AbortController();
    const timeoutHandle = setTimeout(() => controller.abort(), timeoutMs);
    try {
        return await fetch(url, { ...init, signal: controller.signal });
    }
    finally {
        clearTimeout(timeoutHandle);
    }
};
const runAIDiagnostics = async () => {
    const parseURL = buildAIParserURLForDiag();
    const baseURL = String(config.aiParserBaseUrl || '').trim().replace(/\/+$/, '');
    const healthURL = baseURL ? `${baseURL}/healthz` : '';
    const timeoutMs = Number(config.aiParserTimeoutMs) || 8000;
    const enabled = Boolean(config.aiParserFallbackEnabled);
    const results = {
        enabled,
        parseURL,
        healthURL,
        timeoutMs,
        minConfidence: config.aiParserMinConfidence,
        textEnabled: config.aiParserTextEnabled,
        apiKeyHeader: config.aiParserApiKeyHeader || 'X-API-Key',
        health: { ok: false, status: null, detail: '' },
        parse: { ok: false, status: null, detail: '' }
    };

    if (!enabled) {
        results.health.detail = 'fallback deshabilitado por config';
        results.parse.detail = 'fallback deshabilitado por config';
        return results;
    }
    if (!parseURL || !healthURL) {
        results.health.detail = 'URL del parser no configurada';
        results.parse.detail = 'URL del parser no configurada';
        return results;
    }

    try {
        const healthRes = await fetchWithTimeout(healthURL, { method: 'GET' }, timeoutMs);
        results.health.status = healthRes.status;
        const raw = await healthRes.text();
        const detail = String(raw || '').replace(/\s+/g, ' ').trim().slice(0, 220);
        results.health.ok = healthRes.ok;
        results.health.detail = detail || '(sin body)';
    }
    catch (error) {
        results.health.detail = formatDiagError(error);
    }

    try {
        const headers = { 'Content-Type': 'application/json' };
        if (config.aiParserApiKey) {
            headers[config.aiParserApiKeyHeader || 'X-API-Key'] = config.aiParserApiKey;
        }
        const body = JSON.stringify({
            text: 'pague 1234 en supermercado hoy',
            context_date: new Date().toISOString()
        });
        const parseRes = await fetchWithTimeout(parseURL, { method: 'POST', headers, body }, timeoutMs);
        results.parse.status = parseRes.status;
        const raw = await parseRes.text();
        const detail = String(raw || '').replace(/\s+/g, ' ').trim().slice(0, 220);
        results.parse.ok = parseRes.ok;
        results.parse.detail = detail || '(sin body)';
    }
    catch (error) {
        results.parse.detail = formatDiagError(error);
    }

    return results;
};
const extractLinkCodeFromStartPayload = (payload) => {
    const raw = String(payload || '').trim();
    if (!raw)
        return '';
    const normalized = raw.toLowerCase();
    if (!normalized.startsWith('vincular_'))
        return '';
    return normalizeLinkCode(raw.slice('vincular_'.length));
};
const extractStartPayloadFromCommandText = (text) => {
    const raw = String(text || '').trim();
    if (!raw)
        return '';
    const parts = raw.split(/\s+/, 2);
    if (parts.length < 2)
        return '';
    if (!parts[0].toLowerCase().startsWith('/start'))
        return '';
    return String(parts[1] || '').trim();
};
const buildBotTags = (ruleTags) => {
    const safeRuleTags = Array.isArray(ruleTags)
        ? ruleTags.filter((tag) => typeof tag === 'string' && tag.trim().length > 0)
        : [];
    return Array.from(new Set([...safeRuleTags, 'telegram_bot']));
};
export const buildBot = () => {
    const bot = new Telegraf(config.telegramBotToken);
    const consumeLinkCodeForCtx = async (ctx, codeInput) => {
        if (!multiUserLinkingEnabled) {
            await ctx.reply('La vinculacion por codigo no esta habilitada en este entorno.');
            return;
        }
        const code = normalizeLinkCode(codeInput);
        if (!code) {
            await ctx.reply('Uso correcto: /vincular TU-CODIGO');
            return;
        }
        try {
            await expenselogClient.consumeLinkCode({
                code,
                telegram_user_id: ctx.from.id,
                telegram_username: ctx.from.username
            });
            linkStatusCache.set(ctx.from.id, {
                expiresAt: Date.now() + LINK_STATUS_CACHE_TTL_MS,
                value: { ok: true, reason: 'ok' }
            });
            await ctx.reply('Listo. Tu cuenta quedo vinculada. Ya puedes enviar comprobantes para cargarlos en ExpenseLog.');
        }
        catch (error) {
            if (error instanceof ExpenseLogAPIError) {
                await ctx.reply(mapLinkErrorToMessage(error));
                return;
            }
            logger.error('telegram_link_failed', { error, telegramUserId: ctx.from.id });
            await ctx.reply('No pude completar la vinculacion ahora. Intenta de nuevo en unos minutos.');
        }
    };
    const processTextTransactionForCtx = async (ctx, rawText, origin) => {
        if (!canUseAIParser()) {
            await ctx.reply('El parser AI para texto no esta habilitado en este entorno.');
            return;
        }
        await ctx.reply('Recibido. Estoy analizando tu transaccion...');
        const telegramUserId = BigInt(ctx.from.id);
        const telegramMeta = {
            chat_id: ctx.chat.id,
            message_id: ctx.message.message_id,
            file_id: `text:${ctx.message.message_id}`,
            file_unique_id: `text:${ctx.from.id}:${ctx.message.message_id}`
        };
        try {
            const parsed = await parseWithAIParser({
                text: rawText,
                fileType: 'text',
                telegramMeta,
                nativeResult: null
            });
            parsed.source_app = normalizePaymentMethod(parsed.source_app);
            let rulesDb = [];
            try {
                rulesDb = await prisma.userRule.findMany({
                    where: { telegramUserId, enabled: true },
                    orderBy: { priority: 'asc' }
                });
            }
            catch (error) {
                logger.warn('user_rules_load_failed', { telegramUserId: String(telegramUserId), error });
            }
            parsed.rule_output = applyRules(parsed, rulesDb.map((r) => ({
                enabled: r.enabled,
                priority: r.priority,
                when: r.whenJson,
                then: r.thenJson
            })));
            parseResults.inc({ status: 'ok_text_ai' });
            const probableDuplicate = parsed.reference
                ? await prisma.receiptDraft.findFirst({
                    where: {
                        telegramUserId,
                        reference: parsed.reference,
                        status: 'confirmed'
                    }
                })
                : await findFallbackDuplicateByTimeWindow(telegramUserId, parsed);
            const draft = await prisma.receiptDraft.create({
                data: {
                    telegramUserId,
                    chatId: BigInt(ctx.chat.id),
                    messageId: ctx.message.message_id,
                    fileId: `text:${ctx.message.message_id}`,
                    fileUniqueId: `text:${ctx.from.id}:${ctx.message.message_id}`,
                    fileType: 'text',
                    status: mustHaveRequired(parsed) ? 'awaiting_confirm' : 'awaiting_fix',
                    parseResultJson: parsed,
                    dedupeKey: dedupeKey(parsed),
                    reference: parsed.reference
                }
            });
            if (probableDuplicate) {
                await ctx.reply('Detecte una posible carga duplicada. Quieres crearla igual?', dedupeKeyboard());
            }
            await ctx.reply(draftSummary(parsed), { parse_mode: 'Markdown', ...mainDecisionKeyboard() });
            logger.info('draft_created_from_text', { id: draft.id, origin });
        }
        catch (error) {
            parseResults.inc({ status: 'fail_text_ai' });
            logger.warn('text_ai_parse_failed', { error, origin });
            await ctx.reply('No pude procesar ese texto con parser AI. Prueba reformularlo o envia una imagen/PDF.');
        }
    };
    bot.start(async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        const rawStartPayload = String(ctx.startPayload || extractStartPayloadFromCommandText(String(ctx.message?.text || ''))).trim();
        const startPayloadCode = extractLinkCodeFromStartPayload(rawStartPayload);
        if (startPayloadCode) {
            await consumeLinkCodeForCtx(ctx, startPayloadCode);
            return;
        }
        await ctx.reply('Hola. Puedo cargar gastos e ingresos desde comprobantes.\n' +
            'Si es tu primera vez, primero vincula tu cuenta con: /vincular TU-CODIGO\n' +
            'Luego envia una imagen o un PDF del comprobante.');
    });
    bot.help(async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        await ctx.reply('Comandos disponibles:\n' +
            '/vincular TU-CODIGO -> Vincula Telegram con tu cuenta Premium.\n\n' +
            '/cargar TEXTO -> Carga un movimiento desde texto libre.\n\n' +
            '/diag_ai -> Diagnostica conexion con parser AI.\n\n' +
            'Flujo recomendado:\n' +
            '1) Enviar comprobante.\n' +
            '2) Revisar el resumen.\n' +
            '3) Confirmar o corregir datos.');
    });
    bot.command('diag_ai', async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        await ctx.reply('Ejecutando diagnostico AI (health + parse)...');
        const diag = await runAIDiagnostics();
        const parseURL = diag.parseURL || '(no configurada)';
        const healthURL = diag.healthURL || '(no configurada)';
        const message = [
            'Diagnostico AI parser',
            `enabled: ${diag.enabled}`,
            `base health URL: ${healthURL}`,
            `parse URL: ${parseURL}`,
            `timeoutMs: ${diag.timeoutMs}`,
            `minConfidence: ${diag.minConfidence}`,
            `textEnabled: ${diag.textEnabled}`,
            `apiKeyHeader: ${diag.apiKeyHeader}`,
            `healthz: ${diag.health.ok ? 'OK' : 'FAIL'}${diag.health.status != null ? ` (status ${diag.health.status})` : ''}`,
            `health detail: ${diag.health.detail || '-'}`,
            `parse: ${diag.parse.ok ? 'OK' : 'FAIL'}${diag.parse.status != null ? ` (status ${diag.parse.status})` : ''}`,
            `parse detail: ${diag.parse.detail || '-'}`
        ].join('\n');
        await ctx.reply(message);
    });
    bot.command('vincular', async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        const text = String(ctx.message?.text || '').trim();
        const parts = text.split(/\s+/);
        const code = String(parts[1] || '').trim();
        await consumeLinkCodeForCtx(ctx, code);
    });
    bot.command('cargar', async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        const text = String(ctx.message?.text || '').trim();
        const match = text.match(/^\/cargar(?:@\w+)?\s+([\s\S]+)/i);
        const payload = String(match?.[1] || '').trim();
        if (!payload) {
            await ctx.reply('Uso: /cargar <descripcion de la transaccion>\nEjemplo: /cargar pague 1600 en super hoy 20:55');
            return;
        }
        if (!(await requireLinkedPremium(ctx)))
            return;
        await processTextTransactionForCtx(ctx, payload, 'command_cargar');
    });
    bot.on('text', async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        const rawText = String(ctx.message?.text || '').trim();
        if (rawText.startsWith('/vincular'))
            return;
        if (/^\/cargar(?:@\w+)?(?:\s+|$)/i.test(rawText))
            return;
        if (!(await requireLinkedPremium(ctx)))
            return;
        const field = pendingFix.get(ctx.from.id);
        if (!field) {
            if (!config.aiParserTextEnabled) {
                return;
            }
            await processTextTransactionForCtx(ctx, rawText, 'plain_text');
            return;
        }
        const telegramUserId = BigInt(ctx.from.id);
        const draft = await latestDraftByUser(telegramUserId);
        if (!draft)
            return;
        const parsed = toReceiptParseResult(draft.parseResultJson);
        const value = rawText;
        if (field === 'amount')
            parsed.amount = Number(value.replace(',', '.'));
        else if (field === 'datetime_iso')
            parsed.datetime_iso = value;
        else if (field === 'counterparty')
            parsed.counterparty = value;
        else if (field === 'type') {
            const parsedType = parseTypeInput(value);
            if (!parsedType) {
                await ctx.reply('No entendi el tipo. Escribe "gasto" o "ingreso".');
                return;
            }
            parsed.type = parsedType;
        }
        else if (field === 'source_app') {
            const parsedMethod = parsePaymentMethodInput(value);
            if (!parsedMethod) {
                await ctx.reply('No entendi el metodo. Escribe: "transferencia", "efectivo" o "tarjeta credito".');
                return;
            }
            parsed.source_app = parsedMethod;
        }
        else if (field === 'motive')
            parsed.motive = value;
        parsed.source_app = normalizePaymentMethod(parsed.source_app);
        await prisma.receiptDraft.update({
            where: { id: draft.id },
            data: { parseResultJson: parsed, status: 'awaiting_confirm' }
        });
        pendingFix.delete(ctx.from.id);
        draftResults.inc({ status: 'corrected' });
        await ctx.reply(draftSummary(parsed), { parse_mode: 'Markdown', ...mainDecisionKeyboard() });
    });
    bot.on(['photo', 'document'], async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        if (!(await requireLinkedPremium(ctx)))
            return;
        receiptsReceived.inc();
        const stopTotal = stageLatency.startTimer({ stage: 'handler_receipt_total' });
        const message = ctx.message;
        const doc = message?.document;
        const photo = message?.photo?.at(-1);
        const fileType = doc ? 'pdf' : 'image';
        if (doc && doc.mime_type !== 'application/pdf') {
            await ctx.reply('Si envias como documento, debe ser PDF. Si prefieres, envialo como imagen.');
            return;
        }
        const fileId = doc?.file_id ?? photo?.file_id;
        const fileUniqueId = doc?.file_unique_id ?? photo?.file_unique_id;
        if (!fileId || !fileUniqueId)
            return;
        const link = await ctx.telegram.getFileLink(fileId);
        const ext = fileType === 'pdf' ? 'pdf' : 'jpg';
        const tempPath = await tempPathFor(ext);
        try {
            const stopDownload = stageLatency.startTimer({ stage: 'telegram_download' });
            try {
                const res = await fetch(link.toString());
                const buff = Buffer.from(await res.arrayBuffer());
                await writeFile(tempPath, buff);
            }
            finally {
                stopDownload();
            }
            const telegramMeta = {
                chat_id: ctx.chat.id,
                message_id: ctx.message.message_id,
                file_id: fileId,
                file_unique_id: fileUniqueId
            };
            const processed = await processReceipt({
                filePath: tempPath,
                fileType,
                telegramMeta,
                onFallbackAttempt: async () => {
                    await ctx.reply('No pude interpretar completamente el comprobante con el parser interno. Estoy probando parser AI como respaldo, espera unos segundos...');
                }
            });
            const parsed = processed?.result ?? processed;
            const fallbackInfo = processed?.fallback;
            parsed.source_app = normalizePaymentMethod(parsed.source_app);
            const telegramUserId = BigInt(ctx.from.id);
            let rulesDb = [];
            try {
                rulesDb = await prisma.userRule.findMany({
                    where: { telegramUserId, enabled: true },
                    orderBy: { priority: 'asc' }
                });
            }
            catch (error) {
                logger.warn('user_rules_load_failed', { telegramUserId: String(telegramUserId), error });
            }
            parsed.rule_output = applyRules(parsed, rulesDb.map((r) => ({
                enabled: r.enabled,
                priority: r.priority,
                when: r.whenJson,
                then: r.thenJson
            })));
            if (fallbackInfo?.attempted && fallbackInfo?.used) {
                await ctx.reply('Listo, el parser AI devolvio una propuesta. Revisala y confirma.');
            }
            if (fallbackInfo?.attempted && !fallbackInfo?.used && String(fallbackInfo?.reason || '').includes('ai_failed')) {
                await ctx.reply('Intente parser AI como respaldo pero no respondio a tiempo. Te muestro la mejor lectura disponible para que la corrijas si hace falta.');
            }
            const parseStatus = fallbackInfo?.used
                ? 'ok_fallback_ai'
                : (fallbackInfo?.attempted ? 'ok_fallback_native' : 'ok');
            parseResults.inc({ status: parseStatus });
            const probableDuplicate = parsed.reference
                ? await prisma.receiptDraft.findFirst({
                    where: {
                        telegramUserId,
                        reference: parsed.reference,
                        status: 'confirmed'
                    }
                })
                : await findFallbackDuplicateByTimeWindow(telegramUserId, parsed);
            const draft = await prisma.receiptDraft.create({
                data: {
                    telegramUserId,
                    chatId: BigInt(ctx.chat.id),
                    messageId: ctx.message.message_id,
                    fileId,
                    fileUniqueId,
                    fileType,
                    status: mustHaveRequired(parsed) ? 'awaiting_confirm' : 'awaiting_fix',
                    parseResultJson: parsed,
                    dedupeKey: dedupeKey(parsed),
                    reference: parsed.reference
                }
            });
            if (probableDuplicate) {
                await ctx.reply('Detecte una posible carga duplicada. Quieres crearla igual?', dedupeKeyboard());
            }
            await ctx.reply(draftSummary(parsed), { parse_mode: 'Markdown', ...mainDecisionKeyboard() });
            logger.info('draft_created', { id: draft.id });
        }
        catch (error) {
            parseResults.inc({ status: 'fail' });
            logger.error('processing_failed', { error });
            await ctx.reply('No pude leer bien el comprobante. Prueba con otra imagen mas nitida o envia el PDF.');
        }
        finally {
            await safeDelete(tempPath);
            stopTotal();
        }
    });
    bot.action('fix_menu', async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        if (!(await requireLinkedPremium(ctx)))
            return;
        await answerCallback(ctx);
        await ctx.editMessageReplyMarkup(fixMenuKeyboard().reply_markup);
    });
    bot.action('back_summary', async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        if (!(await requireLinkedPremium(ctx)))
            return;
        await answerCallback(ctx);
        await ctx.editMessageReplyMarkup(mainDecisionKeyboard().reply_markup);
    });
    for (const { field, action } of [
        { field: 'amount', action: 'amount' },
        { field: 'datetime_iso', action: 'datetime' },
        { field: 'counterparty', action: 'counterparty' },
        { field: 'type', action: 'type' },
        { field: 'source_app', action: 'source' },
        { field: 'motive', action: 'motive' }
    ]) {
        bot.action(`fix_${action}`, async (ctx) => {
            if (!(await ensurePrivateAllowed(ctx)))
                return;
            if (!(await requireLinkedPremium(ctx)))
                return;
            await answerCallback(ctx);
            pendingFix.set(ctx.from.id, field);
            const prompt = fieldPromptByKey[field];
            const hintLine = prompt?.hint ? `\n${prompt.hint}` : '';
            await ctx.reply(`Escribe el nuevo valor para ${prompt?.label ?? 'este campo'}.${hintLine}`);
        });
    }
    bot.action('reject', async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        if (!(await requireLinkedPremium(ctx)))
            return;
        await answerCallback(ctx);
        const draft = await latestDraftByUser(BigInt(ctx.from.id));
        if (!draft)
            return;
        await prisma.receiptDraft.update({ where: { id: draft.id }, data: { status: 'rejected' } });
        draftResults.inc({ status: 'rejected' });
        await ctx.reply('Listo, descarte este borrador.');
    });
    bot.action('dedupe_cancel', async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        if (!(await requireLinkedPremium(ctx)))
            return;
        await answerCallback(ctx);
        const draft = await latestDraftByUser(BigInt(ctx.from.id));
        if (draft) {
            await prisma.receiptDraft.update({ where: { id: draft.id }, data: { status: 'rejected' } });
            draftResults.inc({ status: 'rejected' });
        }
        await ctx.reply('Perfecto, no cree la carga duplicada.');
    });
    const confirmHandler = async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        if (!(await requireLinkedPremium(ctx, true)))
            return;
        await answerCallback(ctx);
        const draft = await latestDraftByUser(BigInt(ctx.from.id));
        if (!draft)
            return;
        const parsed = toReceiptParseResult(draft.parseResultJson);
        if (!mustHaveRequired(parsed)) {
            const missing = getMissingRequiredLabels(parsed);
            await ctx.reply(`Faltan datos obligatorios: ${missing.join(', ')}.\nToca "Corregir datos" para completarlos.`);
            return;
        }
        const stopCreateTx = stageLatency.startTimer({ stage: 'expenselog_create' });
        let created;
        const normalizedSource = normalizePaymentMethod(parsed.source_app);
        parsed.source_app = normalizedSource;
        const transactionTags = buildBotTags(parsed.rule_output?.tags);
        try {
            if (multiUserLinkingEnabled) {
                created = await expenselogClient.createBotExpense({
                    telegram_user_id: ctx.from.id,
                    type: parsed.type,
                    amount: parsed.amount,
                    currency: 'ARS',
                    datetime_iso: parsed.datetime_iso,
                    counterparty: parsed.counterparty,
                    reference: parsed.reference,
                    motive: parsed.motive,
                    category: parsed.rule_output?.category,
                    tags: transactionTags,
                    provider: normalizedSource,
                    source_meta: parsed.telegram_meta
                });
            }
            else {
                created = await expenselogClient.createTransaction({
                    type: parsed.type,
                    amount: parsed.amount,
                    currency: 'ARS',
                    datetime_iso: parsed.datetime_iso,
                    counterparty: parsed.counterparty,
                    reference: parsed.reference,
                    motive: parsed.motive,
                    category: parsed.rule_output?.category,
                    tags: transactionTags,
                    provider: normalizedSource,
                    source_meta: parsed.telegram_meta
                });
            }
        }
        catch (error) {
            if (error instanceof ExpenseLogAPIError) {
                if (error.code === 'not_linked') {
                    linkStatusCache.set(ctx.from.id, { expiresAt: Date.now() + LINK_STATUS_CACHE_TTL_MS, value: { ok: false, reason: 'not_linked' } });
                    await ctx.reply('Tu cuenta ya no esta vinculada. Genera un nuevo codigo en ExpenseLog y usa /vincular TU-CODIGO.');
                    return;
                }
                if (error.code === 'premium_required') {
                    linkStatusCache.set(ctx.from.id, { expiresAt: Date.now() + LINK_STATUS_CACHE_TTL_MS, value: { ok: false, reason: 'premium_required' } });
                    await ctx.reply('Tu cuenta necesita Premium activo para confirmar comprobantes.');
                    return;
                }
                await ctx.reply(error.message || 'No pude crear la transaccion en ExpenseLog.');
                return;
            }
            logger.error('expenselog_create_failed', { error });
            await ctx.reply('No pude crear la transaccion en ExpenseLog.');
            return;
        }
        finally {
            stopCreateTx();
        }
        await prisma.receiptDraft.update({
            where: { id: draft.id },
            data: {
                status: 'confirmed',
                expenselogTransactionId: created.transaction_id
            }
        });
        draftResults.inc({ status: 'confirmed' });
        await ctx.reply(`Listo, transaccion creada.\nID: ${created.transaction_id}\n${created.url ?? ''}`);
    };
    bot.action('confirm', confirmHandler);
    bot.action('dedupe_create_anyway', confirmHandler);
    return bot;
};
