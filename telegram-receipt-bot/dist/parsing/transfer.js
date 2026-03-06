import { parseArDateTime } from './date.js';
const pick = (text, regex) => text.match(regex)?.[1]?.trim();
const pickLast = (text, regex) => {
    const matches = [...text.matchAll(regex)];
    const last = matches.at(-1);
    return last?.[1]?.trim();
};
const normalizeName = (v) => v?.toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g, '').trim();
const sanitizeExcerpt = (raw) => raw.replace(/\b\d{10,22}\b/g, '[REDACTED_LONG_NUMBER]').slice(0, 300);
export const parseTransferReceipt = (text, aliases, telegramMeta) => {
    const clean = text.replace(/\u00B7/g, ' ').replace(/\s+/g, ' ').trim();
    const amountRaw = pick(clean, /\$\s*([\d\.,]+)/)?.replace(/\./g, '').replace(',', '.');
    const dateIso = parseArDateTime(pick(clean, /Comprobante\s+(\d{2}\/\d{2}\/\d{4}\s*(?:[·.-]?\s*)?\d{2}:\d{2})/i)
        ?? pick(clean, /Fecha de ejecuci[oó]n\s+(\d{2}\/\d{2}\/\d{4}(?:\s+\d{2}:\d{2})?)/i)
        ?? pick(clean, /(\d{1,2}\s+de\s+[a-záéíóú]+\s+de\s+\d{4}\s+a las\s+\d{2}:\d{2})/i)
        ?? '');
    const source = pickLast(clean, /(?:^|\s)De[:\s]+(.+?)\s+CUIT(?:\/CUIL)?/g)
        ?? pickLast(clean, /de:\s+(.+?)\s+cuit(?:\/cuil)?/gi)
        ?? pick(clean, /Origen\s+(.+?)\s+Destino/i);
    const dest = pickLast(clean, /(?:^|\s)Para[:\s]+(.+?)\s+CUIT(?:\/CUIL)?/g)
        ?? pickLast(clean, /para:\s+(.+?)\s+cuit(?:\/cuil)?/gi)
        ?? pick(clean, /Destinatario\s+(.+?)\s+Fecha de ejecuci[oó]n/i)
        ?? pick(clean, /Destino\s+(.+?)\s+(?:Cuenta|Banco|Destinatario|Fecha)/i);
    const reference = pick(clean, /N[uú]mero de operaci[oó]n(?: de [A-Za-z ]+)?\s+([A-Z0-9]+)/i)
        ?? pick(clean, /\bNo\s+(\d{6,})\b/i)
        ?? pick(clean, /Comprobante\s+(\d{8,})/i);
    const motive = pick(clean, /Motivo\s*:\s*(.+?)\s+De\s+/i)
        ?? pick(clean, /Concepto\s+(.+?)\s+Descripci[oó]n/i)
        ?? pick(clean, /Descripci[oó]n\s+(.+?)\s+Aviso/i);
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
        source_app: /mercado pago/i.test(clean) ? 'WALLET' : 'BANK',
        type,
        amount: amountRaw ? Number(amountRaw) : undefined,
        currency: 'ARS',
        datetime_iso: dateIso,
        counterparty,
        reference,
        motive,
        confidence: {
            type: type ? 0.8 : 0.2,
            amount: amountRaw ? 0.9 : 0.2,
            datetime_iso: dateIso ? 0.75 : 0.2,
            counterparty: counterparty ? 0.75 : 0.1,
            overall: 0.68
        },
        raw_text_excerpt: sanitizeExcerpt(clean),
        telegram_meta: telegramMeta
    };
};
