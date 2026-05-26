import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeTransactionURL } from './client.js';

test('normalizeTransactionURL expands relative paths using web base url', () => {
    assert.equal(normalizeTransactionURL('/app/table', 'https://www.expenselog.com.ar'), 'https://www.expenselog.com.ar/app/table');
});

test('normalizeTransactionURL preserves absolute URLs', () => {
    assert.equal(normalizeTransactionURL('https://www.expenselog.com.ar/app/table?id=123'), 'https://www.expenselog.com.ar/app/table?id=123');
});
