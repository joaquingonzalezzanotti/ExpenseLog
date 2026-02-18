const test = require("node:test");
const assert = require("node:assert/strict");
const AlertUI = require("./alerts_ui.js");

test("buildLiquidityAlertKey arma una clave estable", () => {
    const key = AlertUI.buildLiquidityAlertKey({
        recurringId: "rec-1",
        dueDate: "2026-02-20T00:00:00Z",
        kind: "risk_4d",
    });
    assert.equal(key, "rec-1|2026-02-20T00:00:00Z|risk_4d");
});

test("filterVisibleAlerts oculta alerta borrada fuera de ventana 24h", () => {
    const alert = { recurringId: "rec-1", dueDate: "2026-02-20", kind: "preview_7d", daysUntil: 5 };
    const dismissMap = {
        [AlertUI.buildLiquidityAlertKey(alert)]: { dismissedAt: Date.now(), daysUntil: 5 },
    };
    const visible = AlertUI.filterVisibleAlerts([alert], dismissMap, 1);
    assert.equal(visible.length, 0);
});

test("filterVisibleAlerts reaparece al entrar en ventana 24h", () => {
    const alert = { recurringId: "rec-1", dueDate: "2026-02-20", kind: "risk_4d", daysUntil: 1 };
    const dismissMap = {
        [AlertUI.buildLiquidityAlertKey(alert)]: { dismissedAt: Date.now() - 3600_000, daysUntil: 3 },
    };
    const visible = AlertUI.filterVisibleAlerts([alert], dismissMap, 1);
    assert.equal(visible.length, 1);
});

test("summarizeAlerts prioriza panel critico cuando existe una critica", () => {
    const summary = AlertUI.summarizeAlerts([
        { severity: "info" },
        { severity: "critical" },
        { severity: "info" },
    ]);
    assert.equal(summary.total, 3);
    assert.equal(summary.criticalCount, 1);
    assert.equal(summary.infoCount, 2);
    assert.equal(summary.panelSeverity, "critical");
});

