import test from 'node:test';
import assert from 'node:assert/strict';
import {
    AUTO_TEXT_RECEIPT_SUPPRESSION_MS,
    buildReceiptConversationKey,
    getRecentReceiptActivity,
    markRecentReceiptActivity,
    shouldSuppressAutoTextFromRecentReceipt
} from './text_routing.js';

test('buildReceiptConversationKey scopes by user and chat', () => {
    assert.equal(buildReceiptConversationKey(12n, 34n), '12:34');
});

test('recent receipt activity is returned for the same conversation inside the suppression window', () => {
    const store = new Map();
    markRecentReceiptActivity(store, {
        telegramUserId: 99n,
        chatId: 77n,
        messageId: 501,
        phase: 'processing',
        atMs: 1_000
    });

    const recent = getRecentReceiptActivity(store, {
        telegramUserId: 99n,
        chatId: 77n,
        nowMs: 1_500
    });

    assert.deepEqual(recent, {
        telegramUserId: '99',
        chatId: '77',
        messageId: 501,
        phase: 'processing',
        atMs: 1_000
    });
});

test('recent receipt activity is isolated per conversation', () => {
    const store = new Map();
    markRecentReceiptActivity(store, {
        telegramUserId: 99n,
        chatId: 77n,
        messageId: 501,
        atMs: 1_000
    });

    const recent = getRecentReceiptActivity(store, {
        telegramUserId: 99n,
        chatId: 78n,
        nowMs: 1_500
    });

    assert.equal(recent, null);
});

test('auto text is suppressed when there is a recent receipt activity or recent media draft', () => {
    const recentActivity = { atMs: 10_000 };
    assert.equal(shouldSuppressAutoTextFromRecentReceipt({
        recentActivity,
        nowMs: 10_500
    }), true);

    assert.equal(shouldSuppressAutoTextFromRecentReceipt({
        recentMediaDraftCreatedAtMs: 20_000,
        nowMs: 20_500
    }), true);
});

test('auto text is not suppressed for stale receipt activity, explicit commands or pending fixes', () => {
    const staleAtMs = 50_000;
    assert.equal(shouldSuppressAutoTextFromRecentReceipt({
        recentActivity: { atMs: staleAtMs },
        nowMs: staleAtMs + AUTO_TEXT_RECEIPT_SUPPRESSION_MS + 1
    }), false);

    assert.equal(shouldSuppressAutoTextFromRecentReceipt({
        recentActivity: { atMs: 10_000 },
        nowMs: 10_100,
        isExplicitCommand: true
    }), false);

    assert.equal(shouldSuppressAutoTextFromRecentReceipt({
        recentMediaDraftCreatedAtMs: 10_000,
        nowMs: 10_100,
        hasPendingFix: true
    }), false);
});
