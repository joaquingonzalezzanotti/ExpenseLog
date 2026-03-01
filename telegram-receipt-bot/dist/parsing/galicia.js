import { parseArDateTime } from './date.js';
const pick = (text, regex) => text.match(regex)?.[1]?.trim();
const onlyLast4 = (v) => (v ? v.replace(/\D/g, '').slice(-4) : undefined);
const normalizeName = (v) => v?.toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g, '').trim();
const sanitizeExcerpt = (raw) => raw.replace(/\b\d{10,22}\b/g, '[REDACTED_LONG_NUMBER]').slice(0, 300);
export const parseGaliciaReceipt = (text, aliases, telegramMeta) => {
    const clean = text.replace(/\u00B7/g, ' ').replace(/\s+/g, ' ').trim();
    const amountRaw = (pick(clean, /Transferencia enviada\s*\$\s*([\d\.,]+)/i)
        ?? pick(clean, /Monto\s*\$\s*([\d\.,]+)/i))?.replace(/\./g, '').replace(',', '.');
    const dateRaw = pick(clean, /Transferencia enviada\s*\$\s*[\d\.,]+\s+(.+?)\s+De:/i)
        ?? pick(clean, /(\d{2}\/\d{2}\/\d{4}.*?\d{2}:\d{2})/i);
    const dateIso = parseArDateTime(dateRaw ?? '');
    const source = pick(clean, /De:\s+(.+?)\s+CUIT/i);
    const dest = pick(clean, /Para:\s+(.+?)\s+CUIT/i);
    const motive = pick(clean, /Concepto\s+(.+?)\s+N[°ºo]\s+de operaci[oó]n/i);
    const reference = pick(clean, /N[°ºo]\s+de operaci[oó]n\s+([A-Z0-9-]+)/i);
    const fromAcc = pick(clean, /De:.*?Cuenta en\s+(.+?)\s+Para:/i);
    const toAcc = pick(clean, /Para:.*?Cuenta en\s+(.+?)(?:\s+Concepto|$)/i);
    const cbu = pick(clean, /CBU\s+(\d{10,22})/i);
    const aliasNorm = aliases.map((a) => normalizeName(a));
    const srcIsMine = source ? aliasNorm.includes(normalizeName(source)) : false;
    const dstIsMine = dest ? aliasNorm.includes(normalizeName(dest)) : false;
    let type;
    if (srcIsMine && !dstIsMine)
        type = 'expense';
    if (!srcIsMine && dstIsMine)
        type = 'income';
    const counterparty = srcIsMine ? dest : source;
    return {
        source_app: 'BANK',
        type,
        amount: amountRaw ? Number(amountRaw) : undefined,
        currency: 'ARS',
        datetime_iso: dateIso,
        counterparty,
        reference,
        motive,
        account_from: fromAcc ? { bank: fromAcc } : undefined,
        account_to: toAcc ? { bank: toAcc } : undefined,
        cbu_cvu_last4: onlyLast4(cbu),
        confidence: {
            type: type ? 0.85 : 0.2,
            amount: amountRaw ? 0.95 : 0.2,
            datetime_iso: dateIso ? 0.8 : 0.2,
            counterparty: counterparty ? 0.8 : 0.1,
            overall: 0.72
        },
        raw_text_excerpt: sanitizeExcerpt(clean),
        telegram_meta: telegramMeta
    };
};
