const assert = require("node:assert/strict");
const test = require("node:test");
const CashflowUI = require("./cashflow_ui.js");

test("reversion de conciliacion positiva deshace ingresos", () => {
    const buckets = CashflowUI.calculateCashflowBuckets([
        { amount: 100000, currency: "ars", source: "CA" },
        { amount: 5000, currency: "ars", source: "CA", systemOrigin: "reconciliation_adjustment" },
        { amount: -5000, currency: "ars", source: "CA", systemOrigin: "reconciliation_reversal" },
    ], "ars");

    assert.equal(buckets.ars.income, 100000);
    assert.equal(buckets.ars.expense, 0);
});

test("reversion de conciliacion negativa deshace gastos", () => {
    const buckets = CashflowUI.calculateCashflowBuckets([
        { amount: 100000, currency: "ars", source: "CA" },
        { amount: -10000, currency: "ars", source: "CA", systemOrigin: "reconciliation_adjustment" },
        { amount: 10000, currency: "ars", source: "CA", systemOrigin: "reconciliation_reversal" },
    ], "ars");

    assert.equal(buckets.ars.income, 100000);
    assert.equal(buckets.ars.expense, 0);
});

test("movimientos comunes siguen igual por signo", () => {
    const buckets = CashflowUI.calculateCashflowBuckets([
        { amount: 25000, currency: "ars", source: "CA" },
        { amount: 3000, flow: "refund", currency: "ars", source: "CA" },
        { amount: -8000, currency: "ars", source: "CA" },
        { amount: -1000, currency: "ars", source: "EFECTIVO" }, // fuera de CA: ignorado
        { amount: 2000, currency: "usd", source: "CA" },
    ], "ars");

    assert.equal(buckets.ars.income, 25000);
    assert.equal(buckets.ars.refund, 3000);
    assert.equal(buckets.ars.expense, 8000);
    assert.equal(buckets.usd.income, 2000);
    assert.equal(buckets.usd.refund, 0);
    assert.equal(buckets.usd.expense, 0);
});
