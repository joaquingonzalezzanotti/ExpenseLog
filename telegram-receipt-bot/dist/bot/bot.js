import { Telegraf } from 'telegraf';
import { prisma } from '../db/client.js';
import { config, isAllowedTelegramUser } from '../config.js';
import { tempPathFor, safeDelete } from '../processing/file.js';
import { processReceipt } from '../processing/pipeline.js';
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
    source_app: { label: 'origen', hint: 'Ejemplo: MODO' },
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
            'Flujo recomendado:\n' +
            '1) Enviar comprobante.\n' +
            '2) Revisar el resumen.\n' +
            '3) Confirmar o corregir datos.');
    });
    bot.command('vincular', async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        const text = String(ctx.message?.text || '').trim();
        const parts = text.split(/\s+/);
        const code = String(parts[1] || '').trim();
        await consumeLinkCodeForCtx(ctx, code);
    });
    bot.on('text', async (ctx) => {
        if (!(await ensurePrivateAllowed(ctx)))
            return;
        const rawText = String(ctx.message?.text || '').trim();
        if (rawText.startsWith('/vincular'))
            return;
        if (!(await requireLinkedPremium(ctx)))
            return;
        const field = pendingFix.get(ctx.from.id);
        if (!field)
            return;
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
        else if (field === 'motive')
            parsed.motive = value;
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
            const parsed = await processReceipt({ filePath: tempPath, fileType, telegramMeta });
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
            parseResults.inc({ status: 'ok' });
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
    for (const field of ['amount', 'datetime_iso', 'counterparty', 'type', 'motive']) {
        bot.action(`fix_${field === 'datetime_iso' ? 'datetime' : field}`, async (ctx) => {
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
                    provider: parsed.source_app,
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
                    provider: parsed.source_app,
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
