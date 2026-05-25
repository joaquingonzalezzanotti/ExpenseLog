const draftActionPattern = /^([a-z_]+):([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/i;

export const buildDraftAction = (action, draftId) => {
    const safeAction = String(action || '').trim();
    const safeDraftId = String(draftId || '').trim();
    if (!safeAction || !safeDraftId) {
        throw new Error('action and draftId are required');
    }
    return `${safeAction}:${safeDraftId}`;
};

export const parseDraftAction = (value, expectedAction = '') => {
    const raw = String(value || '').trim();
    const match = raw.match(draftActionPattern);
    if (!match) {
        return null;
    }
    const [, action, draftId] = match;
    if (expectedAction && action !== expectedAction) {
        return null;
    }
    return { action, draftId };
};
