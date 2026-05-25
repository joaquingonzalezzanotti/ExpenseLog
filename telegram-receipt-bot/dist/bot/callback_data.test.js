import test from 'node:test';
import assert from 'node:assert/strict';
import { buildDraftAction, parseDraftAction } from './callback_data.js';

test('buildDraftAction encodes action and draft id', () => {
    assert.equal(
        buildDraftAction('confirm', '123e4567-e89b-12d3-a456-426614174000'),
        'confirm:123e4567-e89b-12d3-a456-426614174000'
    );
});

test('parseDraftAction decodes valid callback data', () => {
    assert.deepEqual(
        parseDraftAction('fix_amount:123e4567-e89b-12d3-a456-426614174000'),
        {
            action: 'fix_amount',
            draftId: '123e4567-e89b-12d3-a456-426614174000'
        }
    );
});

test('parseDraftAction validates expected action and rejects invalid values', () => {
    assert.equal(
        parseDraftAction('confirm:123e4567-e89b-12d3-a456-426614174000', 'confirm')?.draftId,
        '123e4567-e89b-12d3-a456-426614174000'
    );
    assert.equal(parseDraftAction('reject:123e4567-e89b-12d3-a456-426614174000', 'confirm'), null);
    assert.equal(parseDraftAction('confirm:not-a-uuid'), null);
});
