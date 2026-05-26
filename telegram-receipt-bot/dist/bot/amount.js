const stripAmountNoise = (raw) => String(raw || '')
    .trim()
    .replace(/\s+/g, '')
    .replace(/[$ARSars]/g, '');

export const parseHumanAmount = (raw) => {
    const clean = stripAmountNoise(raw);
    if (!clean)
        return undefined;
    const normalized = clean.replace(/[^0-9,.-]/g, '');
    if (!normalized || !/[0-9]/.test(normalized))
        return undefined;
    const negative = normalized.startsWith('-');
    const unsigned = normalized.replace(/-/g, '');
    const lastComma = unsigned.lastIndexOf(',');
    const lastDot = unsigned.lastIndexOf('.');
    const hasComma = lastComma >= 0;
    const hasDot = lastDot >= 0;
    let canonical = unsigned;
    if (hasComma && hasDot) {
        const decimalIndex = Math.max(lastComma, lastDot);
        const decimalSeparator = unsigned[decimalIndex];
        const integerPart = unsigned.slice(0, decimalIndex).replace(/[.,]/g, '');
        const decimalPart = unsigned.slice(decimalIndex + 1).replace(/[.,]/g, '');
        canonical = decimalPart ? `${integerPart}.${decimalPart}` : integerPart;
        if (decimalSeparator !== '.' && !decimalPart) {
            canonical = integerPart;
        }
    }
    else if (hasComma || hasDot) {
        const separator = hasComma ? ',' : '.';
        const lastIndex = hasComma ? lastComma : lastDot;
        const parts = unsigned.split(separator);
        const tail = unsigned.slice(lastIndex + 1);
        const treatAsDecimal = parts.length === 2 && tail.length > 0 && tail.length <= 2;
        if (treatAsDecimal) {
            canonical = `${parts[0].replace(/[.,]/g, '')}.${tail.replace(/[.,]/g, '')}`;
        }
        else {
            canonical = unsigned.replace(/[.,]/g, '');
        }
    }
    const parsed = Number(negative ? `-${canonical}` : canonical);
    return Number.isFinite(parsed) ? parsed : undefined;
};
