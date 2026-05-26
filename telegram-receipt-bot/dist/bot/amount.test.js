import test from 'node:test';
import assert from 'node:assert/strict';
import { parseHumanAmount } from './amount.js';

test('parseHumanAmount supports common integer formats', () => {
    assert.equal(parseHumanAmount('1600'), 1600);
    assert.equal(parseHumanAmount('5.000'), 5000);
    assert.equal(parseHumanAmount('5,000'), 5000);
    assert.equal(parseHumanAmount('$ 12.345'), 12345);
});

test('parseHumanAmount supports decimals with local and international separators', () => {
    assert.equal(parseHumanAmount('1600,50'), 1600.5);
    assert.equal(parseHumanAmount('1600.50'), 1600.5);
    assert.equal(parseHumanAmount('5.000,75'), 5000.75);
    assert.equal(parseHumanAmount('5,000.75'), 5000.75);
});

test('parseHumanAmount rejects invalid values', () => {
    assert.equal(parseHumanAmount(''), undefined);
    assert.equal(parseHumanAmount('abc'), undefined);
});
