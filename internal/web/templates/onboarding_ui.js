;(function (root, factory) {
    const api = factory();
    if (typeof module === "object" && module.exports) {
        module.exports = api;
    }
    if (root) {
        root.ExpenseLogOnboarding = api;
    }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
    const HOME_STORAGE_KEY = "expenselog_first_user_onboarding_v1";

    function escapeHTML(value) {
        return String(value || "")
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/"/g, "&quot;")
            .replace(/'/g, "&#039;");
    }

    function readJSON(storageKey, fallbackValue) {
        try {
            const raw = localStorage.getItem(storageKey);
            if (!raw) return fallbackValue;
            const parsed = JSON.parse(raw);
            if (!parsed || typeof parsed !== "object") return fallbackValue;
            return parsed;
        } catch (_) {
            return fallbackValue;
        }
    }

    function writeJSON(storageKey, value) {
        try {
            localStorage.setItem(storageKey, JSON.stringify(value));
        } catch (_) {
            // ignore storage failures
        }
    }

    function readHomeState() {
        const parsed = readJSON(HOME_STORAGE_KEY, { dismissed: false });
        return {
            dismissed: !!parsed.dismissed,
        };
    }

    function writeHomeState(nextState) {
        writeJSON(HOME_STORAGE_KEY, {
            dismissed: !!(nextState && nextState.dismissed),
        });
    }

    function dismissHomeOnboarding() {
        writeHomeState({ dismissed: true });
    }

    function resetHomeOnboarding() {
        writeHomeState({ dismissed: false });
    }

    function shouldShowHomeOnboarding(totalExpenses) {
        const count = Number(totalExpenses) || 0;
        const state = readHomeState();
        return count === 0 && !state.dismissed;
    }

    function getHomeEmptyStateHTML(mode, baseCurrency) {
        if (mode === "first_user") {
            return `
                <div class="empty-state-guide">
                    <strong>Empecemos por tu primer movimiento.</strong>
                    <p>Carga un ingreso y un gasto para activar KPIs, tabla y calendario con datos reales.</p>
                    <div class="empty-state-actions">
                        <button type="button" class="nav-button button-primary" data-onboarding-action="start-income">Cargar ingreso</button>
                        <button type="button" class="nav-button button-secondary" data-onboarding-action="start-expense">Cargar gasto</button>
                        <button type="button" class="nav-button button-secondary" data-onboarding-action="go-table">Ir a tabla</button>
                    </div>
                </div>
            `;
        }

        if (mode === "base_currency") {
            const currency = escapeHTML(String(baseCurrency || "ars").toUpperCase());
            return `
                <div class="empty-state-guide">
                    <strong>No hay gastos en ${currency}.</strong>
                    <p>Puedes cargar un gasto en moneda base o revisar tus movimientos en Tabla.</p>
                    <div class="empty-state-actions">
                        <button type="button" class="nav-button button-primary" data-onboarding-action="start-expense">Cargar gasto</button>
                        <button type="button" class="nav-button button-secondary" data-onboarding-action="go-table">Ver tabla</button>
                    </div>
                </div>
            `;
        }

        return "<strong>Aun no hay gastos.</strong> Carga el primero con Agregar gasto.";
    }

    function buildTableEmptyState(options) {
        const opts = options && typeof options === "object" ? options : {};
        const showAll = !!opts.showAll;
        const isFirstUser = !!opts.isFirstUser;
        const hasFilters = !!opts.hasFilters;

        if (isFirstUser) {
            return `
                <section class="context-empty-state">
                    <h4>Tu tabla se llena con tus primeros movimientos</h4>
                    <p>Carga un ingreso y un gasto para empezar a ver historial, balance acumulado y categorias.</p>
                    <div class="context-empty-actions">
                        <button type="button" class="nav-button button-primary" data-table-empty-action="start-income">Cargar ingreso</button>
                        <button type="button" class="nav-button button-secondary" data-table-empty-action="start-expense">Cargar gasto</button>
                        <button type="button" class="nav-button button-secondary" data-table-empty-action="go-calendar">Ver calendario</button>
                    </div>
                </section>
            `;
        }

        if (hasFilters) {
            return `
                <section class="context-empty-state">
                    <h4>No encontramos movimientos con esos filtros</h4>
                    <p>Prueba limpiar busqueda y filtros para recuperar resultados.</p>
                    <div class="context-empty-actions">
                        <button type="button" class="nav-button button-secondary" data-table-empty-action="clear-filters">Limpiar filtros</button>
                    </div>
                </section>
            `;
        }

        const message = showAll ? "No hay transacciones para mostrar" : "No hay gastos cargados para este mes";
        return `
            <section class="context-empty-state">
                <h4>${escapeHTML(message)}</h4>
                <p>Puedes registrar un movimiento ahora y volver a esta vista.</p>
                <div class="context-empty-actions">
                    <button type="button" class="nav-button button-primary" data-table-empty-action="start-expense">Agregar movimiento</button>
                </div>
            </section>
        `;
    }

    function buildCalendarEmptyState(options) {
        const opts = options && typeof options === "object" ? options : {};
        const isFirstUser = !!opts.isFirstUser;
        const hasFilters = !!opts.hasFilters;
        const title = escapeHTML(String(opts.title || ""));
        const description = escapeHTML(String(opts.description || ""));

        const actions = isFirstUser
            ? `
                <button type="button" class="nav-button button-primary" data-table-empty-action="start-income">Cargar ingreso</button>
                <button type="button" class="nav-button button-secondary" data-table-empty-action="start-expense">Cargar gasto</button>
              `
            : (hasFilters
                ? `<button type="button" class="nav-button button-secondary" data-table-empty-action="clear-filters">Limpiar filtros</button>`
                : `<button type="button" class="nav-button button-primary" data-table-empty-action="start-expense">Agregar movimiento</button>`);

        return `
            <section class="calendar-empty-state">
                <h4>${title}</h4>
                <p>${description}</p>
                <div class="calendar-empty-actions">
                    ${actions}
                </div>
            </section>
        `;
    }

    return {
        readHomeState: readHomeState,
        writeHomeState: writeHomeState,
        dismissHomeOnboarding: dismissHomeOnboarding,
        resetHomeOnboarding: resetHomeOnboarding,
        shouldShowHomeOnboarding: shouldShowHomeOnboarding,
        getHomeEmptyStateHTML: getHomeEmptyStateHTML,
        buildTableEmptyState: buildTableEmptyState,
        buildCalendarEmptyState: buildCalendarEmptyState,
    };
});
