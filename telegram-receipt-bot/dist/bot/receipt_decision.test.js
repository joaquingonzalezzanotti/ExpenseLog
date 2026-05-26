import test from 'node:test';
import assert from 'node:assert/strict';
import { hasBlockingWarnings, shouldAutoConfirmDraft } from './receipt_decision.js';

test('shouldAutoConfirmDraft confirms only complete high-confidence drafts', () => {
    assert.equal(shouldAutoConfirmDraft({
        type: 'expense',
        amount: 5000,
        datetime_iso: '2026-05-26T10:56:00-03:00',
        counterparty: 'Supermercado',
        confidence: { overall: 0.9 }
    }), true);
    assert.equal(shouldAutoConfirmDraft({
        type: 'expense',
        amount: 5000,
        datetime_iso: '2026-05-26T10:56:00-03:00',
        counterparty: 'Supermercado',
        confidence: { overall: 0.69 }
    }), false);
});

test('blocking warnings disable auto confirmation', () => {
    const parsed = {
        type: 'expense',
        amount: 5000,
        datetime_iso: '2026-05-26T10:56:00-03:00',
        counterparty: 'Joaquin Gonzalez Zanotti',
        confidence: { overall: 0.95 },
        warnings: ['Transferencia ambigua: origen y destino parecen ser la misma persona.']
    };
    assert.equal(hasBlockingWarnings(parsed), true);
    assert.equal(shouldAutoConfirmDraft(parsed), false);
});
