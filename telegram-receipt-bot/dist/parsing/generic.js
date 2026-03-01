const sanitizeExcerpt = (raw) => raw.replace(/\b\d{10,22}\b/g, '[REDACTED_LONG_NUMBER]').slice(0, 300);
export const parseGenericReceipt = (text, meta) => {
    const amount = text.match(/\$\s*([\d\.,]+)/)?.[1]?.replace(/\./g, '').replace(',', '.');
    return {
        source_app: 'UNKNOWN',
        currency: 'ARS',
        amount: amount ? Number(amount) : undefined,
        confidence: { overall: 0.3 },
        raw_text_excerpt: sanitizeExcerpt(text),
        telegram_meta: meta
    };
};
