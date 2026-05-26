export const hasRequiredAmount = (amount) => (typeof amount === 'number' && Number.isFinite(amount) && amount !== 0);

export const mustHaveRequired = (parsed) => Boolean(parsed?.type &&
    hasRequiredAmount(parsed.amount) &&
    parsed.datetime_iso &&
    parsed.counterparty);

export const getOverallConfidence = (parsed) => {
    const value = Number(parsed?.confidence?.overall);
    return Number.isFinite(value) ? value : undefined;
};

export const hasBlockingWarnings = (parsed) => Array.isArray(parsed?.warnings) && parsed.warnings.length > 0;

export const shouldAutoConfirmDraft = (parsed, threshold = 0.7) => {
    if (!mustHaveRequired(parsed))
        return false;
    if (hasBlockingWarnings(parsed))
        return false;
    const confidence = getOverallConfidence(parsed);
    return typeof confidence === 'number' && confidence >= threshold;
};
