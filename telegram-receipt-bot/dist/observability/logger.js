const redact = (input) => input.replace(/\b\d{10,22}\b/g, '[REDACTED_LONG_NUMBER]');
const normalizeError = (value) => {
    if (!(value instanceof Error))
        return value;
    return {
        name: value.name,
        message: value.message,
        stack: value.stack
    };
};
const safeJSONStringify = (payload) => {
    const seen = new WeakSet();
    return JSON.stringify(payload, (_key, value) => {
        const normalized = normalizeError(value);
        if (normalized && typeof normalized === 'object') {
            if (seen.has(normalized))
                return '[Circular]';
            seen.add(normalized);
        }
        return normalized;
    });
};
export const logger = {
    info: (msg, data) => console.log(safeJSONStringify({ level: 'info', msg, data })),
    warn: (msg, data) => console.warn(safeJSONStringify({ level: 'warn', msg, data })),
    error: (msg, data) => console.error(safeJSONStringify({ level: 'error', msg, data })),
    sanitizeText: (raw) => redact(raw).slice(0, 500)
};
