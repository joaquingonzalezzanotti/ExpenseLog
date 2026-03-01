import { config } from '../config.js';
import { logger } from '../observability/logger.js';
const isObject = (value) => Boolean(value && typeof value === 'object' && !Array.isArray(value));
const asNumber = (value) => {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : undefined;
};
const pickFirstString = (...values) => {
    for (const value of values) {
        const normalized = String(value ?? '').trim();
        if (normalized)
            return normalized;
    }
    return undefined;
};
const normalizeType = (value) => {
    const normalized = String(value ?? '').trim().toLowerCase();
    if (!normalized)
        return undefined;
    if (['income', 'ingreso', 'entrada'].includes(normalized))
        return 'income';
    if (['refund', 'reintegro'].includes(normalized))
        return 'refund';
    if (['expense', 'gasto', 'egreso', 'salida'].includes(normalized))
        return 'expense';
    return undefined;
};
const normalizeConfidence = (value) => {
    if (typeof value === 'number' && Number.isFinite(value)) {
        return { overall: value };
    }
    if (isObject(value)) {
        const overall = asNumber(value.overall);
        if (typeof overall === 'number') {
            return { ...value, overall };
        }
    }
    return undefined;
};
const sanitizeExcerpt = (raw) => String(raw || '')
    .replace(/\b\d{10,22}\b/g, '[REDACTED_LONG_NUMBER]')
    .slice(0, 300);
const normalizeAIParserResult = (payload, fallbackText, telegramMeta) => {
    const source = isObject(payload?.result)
        ? payload.result
        : isObject(payload?.parse_result)
            ? payload.parse_result
            : isObject(payload?.parsed)
                ? payload.parsed
                : isObject(payload)
                    ? payload
                    : null;
    if (!source) {
        throw new Error('AI parser returned an invalid payload');
    }
    const normalized = {
        source_app: pickFirstString(source.source_app, source.source, source.provider, source.payment_method, source.method),
        type: normalizeType(source.type ?? source.flow ?? source.direction ?? source.movement_type),
        amount: asNumber(source.amount ?? source.total ?? source.monto),
        currency: String(pickFirstString(source.currency, source.moneda) || 'ARS').toUpperCase(),
        datetime_iso: pickFirstString(source.datetime_iso, source.date_time_iso, source.date_time, source.datetime, source.fecha_hora, source.date),
        counterparty: pickFirstString(source.counterparty, source.merchant, source.comercio, source.destination, source.destino, source.recipient),
        reference: pickFirstString(source.reference, source.ref, source.operation_id, source.comprobante),
        motive: pickFirstString(source.motive, source.description, source.concept, source.detalle),
        confidence: normalizeConfidence(source.confidence ?? source.score),
        raw_text_excerpt: pickFirstString(source.raw_text_excerpt, source.excerpt) || sanitizeExcerpt(fallbackText),
        telegram_meta: isObject(source.telegram_meta) ? source.telegram_meta : telegramMeta
    };
    if (isObject(source.rule_output)) {
        normalized.rule_output = source.rule_output;
    }
    return normalized;
};
const buildAIParserURL = () => {
    const base = String(config.aiParserBaseUrl || '').trim().replace(/\/+$/, '');
    const path = String(config.aiParserParsePath || '/parse').startsWith('/')
        ? String(config.aiParserParsePath || '/parse')
        : `/${String(config.aiParserParsePath || '/parse')}`;
    if (!base) {
        return '';
    }
    return `${base}${path}`;
};
export const canUseAIParser = () => {
    return Boolean(config.aiParserFallbackEnabled && buildAIParserURL());
};
export const parseWithAIParser = async ({ text, fileType, telegramMeta, nativeResult }) => {
    const url = buildAIParserURL();
    if (!url) {
        throw new Error('AI parser URL is not configured');
    }
    const controller = new AbortController();
    const timeoutHandle = setTimeout(() => controller.abort(), config.aiParserTimeoutMs);
    try {
        const headers = {
            'Content-Type': 'application/json'
        };
        if (config.aiParserApiKey) {
            headers[config.aiParserApiKeyHeader || 'X-API-Key'] = config.aiParserApiKey;
        }
        const payload = {
            text,
            file_type: fileType,
            telegram_meta: telegramMeta,
            native_result: nativeResult
        };
        const response = await fetch(url, {
            method: 'POST',
            headers,
            body: JSON.stringify(payload),
            signal: controller.signal
        });
        if (!response.ok) {
            throw new Error(`AI parser request failed with status ${response.status}`);
        }
        const data = await response.json();
        return normalizeAIParserResult(data, text, telegramMeta);
    }
    catch (error) {
        logger.warn('ai_parser_request_failed', { error });
        throw error;
    }
    finally {
        clearTimeout(timeoutHandle);
    }
};
