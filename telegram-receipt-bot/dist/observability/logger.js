const redact = (input) => input.replace(/\b\d{10,22}\b/g, '[REDACTED_LONG_NUMBER]');
export const logger = {
    info: (msg, data) => console.log(JSON.stringify({ level: 'info', msg, data })),
    warn: (msg, data) => console.warn(JSON.stringify({ level: 'warn', msg, data })),
    error: (msg, data) => console.error(JSON.stringify({ level: 'error', msg, data })),
    sanitizeText: (raw) => redact(raw).slice(0, 500)
};
