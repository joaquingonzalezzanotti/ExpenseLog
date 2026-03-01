import dotenv from 'dotenv';
dotenv.config();
const csv = (v) => (v ? v.split(',').map((x) => x.trim()).filter(Boolean) : []);
const asNumber = (v, fallback) => {
    const parsed = Number(v);
    return Number.isFinite(parsed) ? parsed : fallback;
};
const asBoolean = (v, fallback) => {
    const normalized = String(v ?? '').trim().toLowerCase();
    if (!normalized)
        return fallback;
    if (['1', 'true', 'yes', 'on'].includes(normalized))
        return true;
    if (['0', 'false', 'no', 'off'].includes(normalized))
        return false;
    return fallback;
};
const parseAdapterMode = (v) => {
    const normalized = String(v || '').trim().toLowerCase();
    if (normalized === 'transactions_api')
        return 'transactions_api';
    return 'expense_api';
};
export const config = {
    telegramBotToken: process.env.TELEGRAM_BOT_TOKEN ?? '',
    allowedTelegramUserIds: csv(process.env.ALLOWED_TELEGRAM_USER_IDS).map((v) => Number(v)),
    telegramAdminUserId: process.env.TELEGRAM_ADMIN_USER_ID ? Number(process.env.TELEGRAM_ADMIN_USER_ID) : undefined,
    myAliases: csv(process.env.MY_ALIASES),
    tzDefault: process.env.TZ_DEFAULT ?? 'America/Argentina/Buenos_Aires',
    databaseUrl: process.env.DATABASE_URL ?? '',
    expenselogApiBaseUrl: process.env.EXPENSELOG_API_BASE_URL ?? '',
    expenselogApiToken: process.env.EXPENSELOG_API_TOKEN ?? '',
    expenselogSessionCookie: process.env.EXPENSELOG_SESSION_COOKIE ?? '',
    expenselogWebBaseUrl: process.env.EXPENSELOG_WEB_BASE_URL ?? '',
    expenselogAdapterMode: parseAdapterMode(process.env.EXPENSELOG_ADAPTER_MODE),
    expenselogTransactionsPath: process.env.EXPENSELOG_TRANSACTIONS_PATH ?? '/api/transactions',
    expenselogExpensePath: process.env.EXPENSELOG_EXPENSE_PATH ?? '/api/expense',
    expenselogDefaultExpenseCategory: process.env.EXPENSELOG_DEFAULT_EXPENSE_CATEGORY ?? 'Varios',
    expenselogDefaultIncomeCategory: process.env.EXPENSELOG_DEFAULT_INCOME_CATEGORY ?? 'Ingresos',
    expenselogDefaultSource: process.env.EXPENSELOG_DEFAULT_SOURCE ?? 'CA',
    expenselogBotInternalSecret: process.env.EXPENSELOG_BOT_INTERNAL_SECRET ?? '',
    expenselogBotConsumeLinkCodePath: process.env.EXPENSELOG_BOT_CONSUME_LINK_CODE_PATH ?? '/api/bot/telegram/consume-link-code',
    expenselogBotLinkStatusPath: process.env.EXPENSELOG_BOT_LINK_STATUS_PATH ?? '/api/bot/telegram/link-status',
    expenselogBotExpensePath: process.env.EXPENSELOG_BOT_EXPENSE_PATH ?? '/api/bot/expense',
    ocrLangs: process.env.OCR_LANGS ?? 'spa+eng',
    pdfRenderMaxPages: asNumber(process.env.PDF_RENDER_MAX_PAGES, 1),
    aiParserBaseUrl: process.env.AI_PARSER_BASE_URL ?? '',
    aiParserParsePath: process.env.AI_PARSER_PARSE_PATH ?? '/parse',
    aiParserApiKey: process.env.AI_PARSER_API_KEY ?? '',
    aiParserApiKeyHeader: process.env.AI_PARSER_API_KEY_HEADER ?? 'X-API-Key',
    aiParserTimeoutMs: asNumber(process.env.AI_PARSER_TIMEOUT_MS, 8000),
    aiParserFallbackEnabled: asBoolean(process.env.AI_PARSER_FALLBACK_ENABLED, true),
    aiParserTextEnabled: asBoolean(process.env.AI_PARSER_TEXT_ENABLED, false),
    aiParserMinConfidence: asNumber(process.env.AI_PARSER_MIN_CONFIDENCE, 0.55),
    logLevel: process.env.LOG_LEVEL ?? 'info',
    port: asNumber(process.env.PORT, 3000)
};
export const isAllowedTelegramUser = (userId) => {
    if (config.allowedTelegramUserIds.length > 0) {
        return config.allowedTelegramUserIds.includes(userId);
    }
    if (config.telegramAdminUserId) {
        return config.telegramAdminUserId === userId;
    }
    return true;
};
