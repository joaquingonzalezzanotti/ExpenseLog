export const AUTO_TEXT_RECEIPT_GRACE_MS = 1500;
export const AUTO_TEXT_RECEIPT_SUPPRESSION_MS = 15000;

export const buildReceiptConversationKey = (telegramUserId, chatId) => (`${telegramUserId}:${chatId}`);

export const pruneRecentReceiptActivity = (store, nowMs = Date.now(), windowMs = AUTO_TEXT_RECEIPT_SUPPRESSION_MS) => {
    for (const [key, value] of store.entries()) {
        if (!value || typeof value.atMs !== 'number' || (nowMs - value.atMs) > windowMs) {
            store.delete(key);
        }
    }
};

export const markRecentReceiptActivity = (store, { telegramUserId, chatId, messageId, phase = 'received', atMs = Date.now() }) => {
    pruneRecentReceiptActivity(store, atMs);
    const key = buildReceiptConversationKey(telegramUserId, chatId);
    const nextValue = {
        telegramUserId: String(telegramUserId),
        chatId: String(chatId),
        messageId: Number(messageId),
        phase: String(phase || 'received'),
        atMs
    };
    store.set(key, nextValue);
    return nextValue;
};

export const getRecentReceiptActivity = (store, { telegramUserId, chatId, nowMs = Date.now(), windowMs = AUTO_TEXT_RECEIPT_SUPPRESSION_MS }) => {
    pruneRecentReceiptActivity(store, nowMs, windowMs);
    const key = buildReceiptConversationKey(telegramUserId, chatId);
    const value = store.get(key);
    if (!value) {
        return null;
    }
    return (nowMs - value.atMs) <= windowMs ? value : null;
};

export const shouldSuppressAutoTextFromRecentReceipt = ({ recentActivity, recentMediaDraftCreatedAtMs, nowMs = Date.now(), suppressionWindowMs = AUTO_TEXT_RECEIPT_SUPPRESSION_MS, hasPendingFix = false, isExplicitCommand = false }) => {
    if (hasPendingFix || isExplicitCommand) {
        return false;
    }
    const candidates = [];
    if (recentActivity && typeof recentActivity.atMs === 'number') {
        candidates.push(recentActivity.atMs);
    }
    if (typeof recentMediaDraftCreatedAtMs === 'number') {
        candidates.push(recentMediaDraftCreatedAtMs);
    }
    return candidates.some((atMs) => {
        const ageMs = nowMs - atMs;
        return ageMs >= 0 && ageMs <= suppressionWindowMs;
    });
};
