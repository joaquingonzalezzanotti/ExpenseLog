const test = require("node:test");
const assert = require("node:assert/strict");
const OnboardingUI = require("./onboarding_ui.js");

function createMemoryStorage() {
    const data = new Map();
    return {
        getItem(key) {
            return data.has(key) ? data.get(key) : null;
        },
        setItem(key, value) {
            data.set(String(key), String(value));
        },
        removeItem(key) {
            data.delete(String(key));
        },
        clear() {
            data.clear();
        },
    };
}

test("shouldShowHomeOnboarding depende de gastos y estado dismiss", () => {
    global.localStorage = createMemoryStorage();
    OnboardingUI.resetHomeOnboarding();
    assert.equal(OnboardingUI.shouldShowHomeOnboarding(0), true);
    OnboardingUI.dismissHomeOnboarding();
    assert.equal(OnboardingUI.shouldShowHomeOnboarding(0), false);
    assert.equal(OnboardingUI.shouldShowHomeOnboarding(3), false);
});

test("empty state home para first_user incluye accion de plantilla", () => {
    const html = OnboardingUI.getHomeEmptyStateHTML("first_user", "ars");
    assert.match(html, /data-onboarding-action="load-template"/);
    assert.match(html, /Cargar 3 ejemplos/);
});

test("empty state tabla para first_user incluye accion de plantilla", () => {
    const html = OnboardingUI.buildTableEmptyState({
        isFirstUser: true,
        hasFilters: false,
        showAll: false,
    });
    assert.match(html, /data-table-empty-action="load-template"/);
    assert.match(html, /Cargar 3 ejemplos/);
});

test("empty state calendario para first_user incluye accion de plantilla", () => {
    const html = OnboardingUI.buildCalendarEmptyState({
        isFirstUser: true,
        hasFilters: false,
        title: "Sin datos",
        description: "Carga tu primer movimiento",
    });
    assert.match(html, /data-table-empty-action="load-template"/);
});

test("base_currency sanitiza la moneda en HTML", () => {
    const html = OnboardingUI.getHomeEmptyStateHTML("base_currency", "<ars>");
    assert.match(html, /No hay gastos en &lt;ARS&gt;/);
});

