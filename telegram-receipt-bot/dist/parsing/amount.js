const normalizeReceiptAmountText = (raw) => String(raw || '')
    .trim()
    .replace(/\s+/g, ' ')
    .replace(/[$]/g, '')
    .replace(/[Il|]/g, '1')
    .replace(/[Oo]/g, '0');

export const parseReceiptAmount = (raw) => {
    const normalized = normalizeReceiptAmountText(raw);
    if (!normalized) {
        return undefined;
    }
    const token = normalized.match(/-?\d[\d.,]*/)?.[0];
    if (!token) {
        return undefined;
    }
    const lastComma = token.lastIndexOf(',');
    const lastDot = token.lastIndexOf('.');
    const hasComma = lastComma >= 0;
    const hasDot = lastDot >= 0;
    let canonical = token;
    if (hasComma && hasDot) {
        const decimalIndex = Math.max(lastComma, lastDot);
        const integerPart = token.slice(0, decimalIndex).replace(/[.,]/g, '');
        const decimalPart = token.slice(decimalIndex + 1).replace(/[.,]/g, '');
        canonical = decimalPart ? `${integerPart}.${decimalPart}` : integerPart;
    }
    else if (hasComma || hasDot) {
        const separator = hasComma ? ',' : '.';
        const lastIndex = hasComma ? lastComma : lastDot;
        const tail = token.slice(lastIndex + 1);
        const treatAsDecimal = tail.length > 0 && tail.length <= 2;
        if (treatAsDecimal) {
            canonical = `${token.slice(0, lastIndex).replace(/[.,]/g, '')}.${tail.replace(/[.,]/g, '')}`;
        }
        else {
            canonical = token.replace(/[.,]/g, '');
        }
    }
    const parsed = Number(canonical);
    return Number.isFinite(parsed) ? parsed : undefined;
};
