const isCardRelated = (r) => {
    const signal = `${r.account_from?.bank ?? ''} ${r.account_to?.bank ?? ''} ${r.motive ?? ''}`;
    return /(tarjeta|visa|master|amex|debito|credito)/i.test(signal);
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
    if (r.type === 'expense' && isCardRelated(r))
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
    `Origen: ${r.source_app}`,
    `Motivo: ${r.motive ?? '-'}`
].join('\n');
