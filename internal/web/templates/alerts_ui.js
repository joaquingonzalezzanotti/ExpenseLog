(function (root, factory) {
    const api = factory();
    if (typeof module === "object" && module.exports) {
        module.exports = api;
    }
    if (root) {
        root.ExpenseLogAlertUI = api;
    }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
    function buildLiquidityAlertKey(item) {
        const recurringId = String(item && item.recurringId ? item.recurringId : "sin-recurring");
        const dueDate = String(item && item.dueDate ? item.dueDate : "");
        const kind = String(item && item.kind ? item.kind : "preview_7d");
        return recurringId + "|" + dueDate + "|" + kind;
    }

    function shouldHideDismissedAlert(item, dismissMap, reappearDays) {
        const map = dismissMap && typeof dismissMap === "object" ? dismissMap : {};
        const days = Number.isFinite(reappearDays) ? reappearDays : 1;
        const key = buildLiquidityAlertKey(item);
        const entry = map[key];
        if (!entry) return false;

        const daysUntil = Number(item && item.daysUntil ? item.daysUntil : 0);
        const dismissedDaysUntil = typeof entry === "object"
            ? Number(entry.daysUntil ?? Number.POSITIVE_INFINITY)
            : Number.POSITIVE_INFINITY;

        const isReappearWindow = daysUntil <= days;
        const wasDismissedBeforeWindow = dismissedDaysUntil > days;
        if (isReappearWindow && wasDismissedBeforeWindow) {
            return false;
        }
        return true;
    }

    function filterVisibleAlerts(alerts, dismissMap, reappearDays) {
        const list = Array.isArray(alerts) ? alerts : [];
        return list.filter(function (item) {
            return !shouldHideDismissedAlert(item, dismissMap, reappearDays);
        });
    }

    function summarizeAlerts(alerts) {
        const list = Array.isArray(alerts) ? alerts : [];
        const criticalCount = list.filter(function (item) {
            return item && item.severity === "critical";
        }).length;
        const infoCount = list.length - criticalCount;
        return {
            total: list.length,
            criticalCount: criticalCount,
            infoCount: infoCount,
            panelSeverity: criticalCount > 0 ? "critical" : "info",
        };
    }

    function resolveRules(data, rules) {
        const incoming = data && typeof data === "object" ? data : {};
        const base = rules && typeof rules === "object" ? rules : {};
        const reappearDays = Number(incoming.reappearDays ?? base.reappearDays ?? 1);
        const windowDays = Number(incoming.windowDays ?? base.windowDays ?? 7);
        const criticalDays = Number(incoming.criticalDays ?? base.criticalDays ?? 4);
        const currency = String(incoming.currency || base.currency || "ars").toLowerCase();
        return {
            reappearDays: Number.isFinite(reappearDays) ? reappearDays : 1,
            windowDays: Number.isFinite(windowDays) ? windowDays : 7,
            criticalDays: Number.isFinite(criticalDays) ? criticalDays : 4,
            currency: currency,
        };
    }

    function buildPanelModel(data, dismissMap, rules) {
        const payload = data && typeof data === "object" ? data : {};
        const effectiveRules = resolveRules(payload, rules);
        const incomingAlerts = Array.isArray(payload.alerts) ? payload.alerts : [];
        const visibleAlerts = filterVisibleAlerts(incomingAlerts, dismissMap, effectiveRules.reappearDays);
        const summary = summarizeAlerts(visibleAlerts);
        const hasCritical = summary.panelSeverity === "critical";
        return {
            rules: effectiveRules,
            alerts: visibleAlerts,
            summary: summary,
            widgetVisible: visibleAlerts.length > 0,
            title: hasCritical ? "Riesgo de saldo" : "Notificaciones de saldo",
            icon: hasCritical ? "warning" : "info",
        };
    }

    return {
        buildLiquidityAlertKey: buildLiquidityAlertKey,
        shouldHideDismissedAlert: shouldHideDismissedAlert,
        filterVisibleAlerts: filterVisibleAlerts,
        summarizeAlerts: summarizeAlerts,
        resolveRules: resolveRules,
        buildPanelModel: buildPanelModel,
    };
});
