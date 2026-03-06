;(function (root, factory) {
    const api = factory();
    if (typeof module === "object" && module.exports) {
        module.exports = api;
    }
    if (root) {
        root.ExpenseLogCashflow = api;
    }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
    function normalizeCurrency(currency, fallback) {
        const value = String(currency || "").trim().toLowerCase();
        if (!value) return String(fallback || "ars").toLowerCase();
        return value;
    }

    function normalizeSource(source) {
        return String(source || "").trim().toUpperCase();
    }

    function toNumber(value) {
        const n = Number(value);
        return Number.isFinite(n) ? n : 0;
    }

    function normalizeZero(value) {
        return Math.abs(value) < 0.000001 ? 0 : value;
    }

    function ensureBucket(map, currency) {
        if (!map[currency]) {
            map[currency] = { income: 0, refund: 0, expense: 0 };
        }
        return map[currency];
    }

    function resolveFlow(expense, amount) {
        const flow = String(expense && expense.flow ? expense.flow : "").trim().toLowerCase();
        if (flow) return flow;
        return amount >= 0 ? "income" : "expense";
    }

    function applyRegularMovement(bucket, amount, flow) {
        if (amount > 0) {
            if (flow === "refund") {
                bucket.refund += amount;
                return;
            }
            bucket.income += amount;
            return;
        }
        if (amount < 0) {
            bucket.expense += Math.abs(amount);
        }
    }

    // Reconciliation reversal is a contra-entry for dashboard cashflow buckets:
    // - If reversal amount is negative, it undoes a previous positive adjustment income.
    // - If reversal amount is positive, it undoes a previous negative adjustment expense.
    function applyReconciliationReversal(bucket, amount) {
        if (amount < 0) {
            bucket.income += amount; // subtract from income
            return;
        }
        if (amount > 0) {
            bucket.expense -= amount; // subtract from expense
        }
    }

    function calculateCashflowBuckets(expenses, fallbackCurrency) {
        const rows = Array.isArray(expenses) ? expenses : [];
        const byCurrency = {};

        rows.forEach(function (exp) {
            const source = normalizeSource(exp && exp.source);
            if (!(source === "" || source === "CA")) return;

            const currency = normalizeCurrency(exp && exp.currency, fallbackCurrency);
            const amount = toNumber(exp && exp.amount);
            const flow = resolveFlow(exp, amount);
            const systemOrigin = String(exp && exp.systemOrigin ? exp.systemOrigin : "").toLowerCase();
            const bucket = ensureBucket(byCurrency, currency);

            if (systemOrigin === "reconciliation_reversal") {
                applyReconciliationReversal(bucket, amount);
                return;
            }

            applyRegularMovement(bucket, amount, flow);
        });

        Object.keys(byCurrency).forEach(function (currency) {
            byCurrency[currency].income = normalizeZero(byCurrency[currency].income);
            byCurrency[currency].refund = normalizeZero(byCurrency[currency].refund);
            byCurrency[currency].expense = normalizeZero(byCurrency[currency].expense);
        });

        return byCurrency;
    }

    return {
        calculateCashflowBuckets: calculateCashflowBuckets,
    };
});

