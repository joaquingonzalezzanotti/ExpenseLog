import { parseArDateTime } from './date.js';
const pick = (text, regex) => text.match(regex)?.[1]?.trim();
const onlyLast4 = (v) => (v ? v.replace(/\D/g, '').slice(-4) : undefined);
const normalizeName = (v) => v?.toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g, '').trim();
const sanitizeExcerpt = (raw) => raw.replace(/\b\d{10,22}\b/g, '[REDACTED_LONG_NUMBER]').slice(0, 300);
export const parseModoReceipt = (text, aliases, telegramMeta) => {
    const clean = text.replace(/S\.E\.U\.O\./gi, ' ').replace(/\s+/g, ' ').trim();
    const source = pick(clean, /Transferencia de\s+(.+?)\s+Desde la cuenta/i);
    const dest = pick(clean, /Para\s+(.+?)\s+A su cuenta/i);
    const paidTo = pick(clean, /Pagaste a\s+(.+?)\s+Monto/i);
    const amountRaw = pick(clean, /Monto\s*\$\s*([\d\.,]+)/i)?.replace(/\./g, '').replace(',', '.');
    const ref = pick(clean, /Ref\.?\s*([A-Z0-9-]{6,})/i);
    const motive = pick(clean, /Motivo\s+(.+?)(?:\s+Fecha y hora|\s+Fecha y Hora|$)/i);
    const dateIso = parseArDateTime(pick(clean, /Fecha y hora\s+(.+?)(?:\s+Medio de pago|\s+Comprobante|\s+ID de QR|$)/i)
        ?? pick(clean, /Fecha y Hora\s+(.+?)(?:\s+Medio de pago|\s+Comprobante|\s+ID de QR|$)/i)
        ?? '');
    const fromAcc = pick(clean, /Desde la cuenta\s+(.+?)\s+Para/i);
    const toAcc = pick(clean, /A su cuenta\s+(.+?)\s+CBU\/CVU/i)
        ?? pick(clean, /Medio de pago\s+(.+?)(?:\s+Comprobante|\s+ID de QR|$)/i);
    const cbu = pick(clean, /CBU\/CVU\s+(\d{10,22})/i);
    const aliasNorm = aliases.map((a) => normalizeName(a));
    const srcIsMine = source && aliasNorm.includes(normalizeName(source));
    const dstIsMine = dest && aliasNorm.includes(normalizeName(dest));
    const paidToNorm = normalizeName(paidTo);
    const paidToIsMine = paidToNorm ? aliasNorm.includes(paidToNorm) : false;
    let type;
    if (paidTo && !paidToIsMine)
        type = 'expense';
    if (srcIsMine && !dstIsMine)
        type = 'expense';
    if (!srcIsMine && dstIsMine)
        type = 'income';
    const counterparty = paidTo || (srcIsMine ? dest : source);
    return {
        source_app: 'MODO',
        type,
        amount: amountRaw ? Number(amountRaw) : undefined,
        currency: 'ARS',
        datetime_iso: dateIso,
        counterparty,
        reference: ref,
        motive,
        account_from: fromAcc ? { bank: fromAcc } : undefined,
        account_to: toAcc ? { bank: toAcc } : undefined,
        cbu_cvu_last4: onlyLast4(cbu),
        confidence: {
            type: type ? 0.9 : 0.2,
            amount: amountRaw ? 0.95 : 0.2,
            datetime_iso: dateIso ? 0.8 : 0.2,
            counterparty: counterparty ? 0.8 : 0.1,
            overall: 0.75
        },
        raw_text_excerpt: sanitizeExcerpt(clean),
        telegram_meta: telegramMeta
    };
};
