const normalizeForMethodMatch = (raw) => String(raw || '')
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .trim()
    .toUpperCase();
const normalizePaymentMethod = (sourceApp) => {
    const raw = normalizeForMethodMatch(sourceApp);
    if (!raw)
        return 'CA';
    if (raw === 'EFECTIVO' || raw.includes('CASH'))
        return 'EFECTIVO';
    if (raw.includes('DEBITO') || raw.includes('DEBIT') || raw.includes('TRANSFER') || raw.includes('BANK') || raw.includes('WALLET') || raw.includes('MODO'))
        return 'CA';
    if (raw === 'TARJETA' || raw.includes('CREDITO') || raw.includes('CREDIT') || raw.includes('MASTERCARD') || raw.includes('AMEX') || raw.includes('VISA')) {
        return 'TARJETA';
    }
    return 'CA';
};
const formatPaymentMethodLabel = (sourceApp) => {
    const code = normalizePaymentMethod(sourceApp);
    if (code === 'TARJETA')
        return 'Tarjeta de credito';
    if (code === 'EFECTIVO')
        return 'Efectivo (solo registro)';
    return 'Transferencia / Debito';
};
const normalizeMoney = (amount) => (typeof amount === 'number' && Number.isFinite(amount)
    ? amount.toLocaleString('es-AR', { minimumFractionDigits: 0, maximumFractionDigits: 2 })
    : '-');
const formatDateTimeLabel = (iso) => {
    const raw = String(iso || '').trim();
    if (!raw)
        return '-';
    const match = raw.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/);
    if (match) {
        const [, yyyy, mm, dd, hh, min] = match;
        return `${dd}/${mm}/${yyyy} ${hh}:${min}`;
    }
    return raw;
};
export const resolveSuggestedCategory = (r) => {
    return r.rule_output?.category;
};
const resolveTypeLabel = (r) => {
    if (r.type === 'income')
        return 'Ingreso';
    if (r.type === 'expense' && normalizePaymentMethod(r.source_app) === 'TARJETA')
        return 'Gasto (tarjeta)';
    if (r.type === 'expense')
        return 'Gasto';
    return 'Pendiente';
};
export const draftSummary = (r) => [
    '*Resumen del comprobante*',
    `Tipo: ${resolveTypeLabel(r)}`,
    `Monto: ${normalizeMoney(r.amount)} ${r.currency}`,
    `Fecha/Hora: ${formatDateTimeLabel(r.datetime_iso)}`,
    `Contraparte (comercio/destino): ${r.counterparty ?? '-'}`,
    `Referencia (comprobante): ${r.reference ?? '-'}`,
    `Categoria (regla): ${resolveSuggestedCategory(r) ?? '-'}`,
    `Metodo: ${formatPaymentMethodLabel(r.source_app)}`,
    `Motivo: ${r.motive ?? '-'}`
].join('\n');
