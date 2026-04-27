;(function (root, factory) {
    const api = factory(root);
    if (typeof module === "object" && module.exports) {
        module.exports = api;
    }
    if (root) {
        root.ExpenseLogAnalytics = api;
    }
})(typeof globalThis !== "undefined" ? globalThis : this, function (root) {
    function toNumber(value) {
        const n = Number(value);
        return Number.isFinite(n) ? n : 0;
    }

    function normalizeCurrency(currency, fallback) {
        const value = String(currency || "").trim().toLowerCase();
        if (!value) return String(fallback || "ars").trim().toLowerCase();
        return value;
    }

    function normalizeSource(source) {
        const code = String(source || "").trim().toUpperCase();
        if (code === "" || code === "CA") return "CA";
        if (code === "TARJETA") return "TARJETA";
        if (code === "EFECTIVO") return "EFECTIVO";
        return "CA";
    }

    function normalizeText(value) {
        return String(value || "").trim().toLowerCase();
    }

    function resolveFlow(expense, amount) {
        const flow = normalizeText(expense && expense.flow);
        if (flow) return flow;
        return amount >= 0 ? "income" : "expense";
    }

    function isExpenseMovement(expense) {
        const amount = toNumber(expense && expense.amount);
        const systemOrigin = normalizeText(expense && (expense.systemOrigin || expense.system_origin));
        return resolveFlow(expense, amount) === "expense" && amount < 0 && systemOrigin !== "reconciliation_reversal";
    }

    function isCardPurchaseMovement(expense) {
        return normalizeSource(expense && expense.source) === "TARJETA" && isExpenseMovement(expense);
    }

    function isCardSettlementMovement(expense) {
        const category = normalizeText(expense && expense.category);
        if (category === "tarjeta por pagar") return true;
        const systemOrigin = normalizeText(expense && (expense.systemOrigin || expense.system_origin));
        return systemOrigin.startsWith("card_payment_");
    }

    function filterMovementsForScope(expenses, includeCreditCard) {
        const rows = Array.isArray(expenses) ? expenses : [];
        return rows.filter(function (expense) {
            if (includeCreditCard) {
                // Keep purchases, but exclude settlements to avoid double counting.
                return !isCardSettlementMovement(expense);
            }
            return !isCardPurchaseMovement(expense) && !isCardSettlementMovement(expense);
        });
    }

    function fallbackCashflowBuckets(expenses, fallbackCurrency) {
        const rows = Array.isArray(expenses) ? expenses : [];
        const byCurrency = {};

        function ensureBucket(currency) {
            if (!byCurrency[currency]) {
                byCurrency[currency] = { income: 0, refund: 0, expense: 0 };
            }
            return byCurrency[currency];
        }

        rows.forEach(function (expense) {
            const source = normalizeSource(expense && expense.source);
            if (!(source === "" || source === "CA")) return;

            const currency = normalizeCurrency(expense && expense.currency, fallbackCurrency);
            const amount = toNumber(expense && expense.amount);
            const flow = resolveFlow(expense, amount);
            const systemOrigin = normalizeText(expense && expense.systemOrigin);
            const bucket = ensureBucket(currency);

            if (systemOrigin === "reconciliation_reversal") {
                if (amount < 0) {
                    bucket.income += amount;
                } else if (amount > 0) {
                    bucket.expense -= amount;
                }
                return;
            }

            if (amount > 0) {
                if (flow === "refund") {
                    bucket.refund += amount;
                } else {
                    bucket.income += amount;
                }
                return;
            }
            if (amount < 0) {
                bucket.expense += Math.abs(amount);
            }
        });

        return byCurrency;
    }

    function calculateCashflowTotals(expenses, fallbackCurrency, targetCurrency) {
        const calculator = root
            && root.ExpenseLogCashflow
            && typeof root.ExpenseLogCashflow.calculateCashflowBuckets === "function"
            ? root.ExpenseLogCashflow.calculateCashflowBuckets
            : fallbackCashflowBuckets;
        const buckets = calculator(expenses, fallbackCurrency);

        if (targetCurrency) {
            const key = normalizeCurrency(targetCurrency, fallbackCurrency);
            const bucket = buckets[key] || { income: 0, refund: 0, expense: 0 };
            return {
                income: toNumber(bucket.income),
                refund: toNumber(bucket.refund),
                expense: toNumber(bucket.expense),
            };
        }

        return Object.values(buckets).reduce(function (acc, bucket) {
            acc.income += toNumber(bucket && bucket.income);
            acc.refund += toNumber(bucket && bucket.refund);
            acc.expense += toNumber(bucket && bucket.expense);
            return acc;
        }, { income: 0, refund: 0, expense: 0 });
    }

    function buildScopedAnalytics(expenses, options) {
        const config = options || {};
        const fallbackCurrency = normalizeCurrency(config.fallbackCurrency, "ars");
        const targetCurrency = config.currency ? normalizeCurrency(config.currency, fallbackCurrency) : "";
        const includeCreditCard = !!config.includeCreditCard;
        const visibleMovements = filterMovementsForScope(expenses, includeCreditCard);
        const expenseMovements = visibleMovements.filter(isExpenseMovement);
        const cashflowTotals = calculateCashflowTotals(visibleMovements, fallbackCurrency, targetCurrency);
        const totalExpenses = expenseMovements.reduce(function (sum, expense) {
            return sum + Math.abs(toNumber(expense && expense.amount));
        }, 0);
        const totalCashSpend = expenseMovements.reduce(function (sum, expense) {
            return normalizeSource(expense && expense.source) === "TARJETA"
                ? sum
                : sum + Math.abs(toNumber(expense && expense.amount));
        }, 0);
        const totalCardSpend = expenseMovements.reduce(function (sum, expense) {
            return isCardPurchaseMovement(expense)
                ? sum + Math.abs(toNumber(expense && expense.amount))
                : sum;
        }, 0);

        return {
            visibleMovements: visibleMovements,
            expenseMovements: expenseMovements,
            totalIncome: cashflowTotals.income,
            totalRefund: cashflowTotals.refund,
            totalInflow: cashflowTotals.income + cashflowTotals.refund,
            totalExpenses: totalExpenses,
            totalCashSpend: totalCashSpend,
            totalCardSpend: totalCardSpend,
        };
    }

    return {
        buildScopedAnalytics: buildScopedAnalytics,
        filterMovementsForScope: filterMovementsForScope,
        isCardPurchaseMovement: isCardPurchaseMovement,
        isCardSettlementMovement: isCardSettlementMovement,
    };
});
