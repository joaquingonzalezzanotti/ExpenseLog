import test from 'node:test';
import assert from 'node:assert/strict';
import { parseModoReceipt } from './modo.js';

test('parseModoReceipt keeps the exact amount from OCR text', () => {
    const parsed = parseModoReceipt('MODO Ref. ABC123456 Transferencia de Joaquin Gonzalez Zanotti Desde la cuenta Santander CA 8536 Para Joaquin Gonzalez Zanotti A su cuenta Brubank CA 0001 Monto $1 Motivo VAR Fecha y hora 26/05/2026 a las 10:56hs', ['Joaquin Gonzalez Zanotti'], {});
    assert.equal(parsed.amount, 1);
    assert.equal(parsed.datetime_iso, '2026-05-26T10:56:00-03:00');
});

test('parseModoReceipt keeps decimal point amounts from card receipts', () => {
    const parsed = parseModoReceipt('MODO Ref. MGQ3QUOL Pagaste a PEROTTI DIEGO MARTIN Monto $20507.5 en 1 cuota Fecha y Hora 28/05/2026 a las 12:46hs Medio de pago Santander Visa Credito 5886', [], {});
    assert.equal(parsed.amount, 20507.5);
    assert.equal(parsed.type, 'expense');
});

test('parseModoReceipt recovers leading one from common OCR confusion', () => {
    const parsed = parseModoReceipt('MODO Ref. NULZ8263 Pagaste a JUAN AGUILERA E HIJOS Monto $I6886 Fecha y Hora 28/05/2026 a las 12:29hs Medio de pago Santander CA 8536', [], {});
    assert.equal(parsed.amount, 16886);
    assert.equal(parsed.type, 'expense');
});

test('parseModoReceipt does not infer expense for same-party transfers', () => {
    const parsed = parseModoReceipt('MODO Ref. ABC123456 Transferencia de Joaquin Gonzalez Zanotti Desde la cuenta Santander CA 8536 Para Joaquin Gonzalez Zanotti A su cuenta Brubank CA 0001 Monto $5000 Motivo VAR Fecha y hora 26/05/2026 a las 10:56hs', ['Joaquin Gonzalez Zanotti'], {});
    assert.equal(parsed.type, undefined);
    assert.match(parsed.warnings.join(' '), /Transferencia ambigua/i);
});
