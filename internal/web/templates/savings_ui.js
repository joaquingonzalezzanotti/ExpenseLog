(function() {
    const savingsSummaryEl = document.getElementById('savingsSummary');
    const secondaryCurrencyBoxEl = document.getElementById('secondaryCurrencyBox');
    const savingsGoalsEl = document.getElementById('savingsGoals');
    const newGoalBtn = document.getElementById('newGoalBtn');
    const modal = document.getElementById('savingsGoalModal');
    const closeModalBtn = document.getElementById('closeGoalModalBtn');
    const cancelGoalBtn = document.getElementById('cancelGoalBtn');
    const goalForm = document.getElementById('savingsGoalForm');
    const goalMessage = document.getElementById('savingsGoalMessage');

    const fmtAmount = (amount, currency) => {
        const normalized = String(currency || 'ars').toLowerCase();
        const locale = normalized === 'usd' ? 'en-US' : 'es-AR';
        return new Intl.NumberFormat(locale, { style: 'currency', currency: normalized.toUpperCase(), maximumFractionDigits: 2 }).format(Number(amount || 0));
    };

    const showGoalModal = () => {
        modal.hidden = false;
        modal.setAttribute('aria-hidden', 'false');
    };

    const hideGoalModal = () => {
        modal.hidden = true;
        modal.setAttribute('aria-hidden', 'true');
        goalMessage.textContent = '';
        goalMessage.className = 'form-message';
        goalForm.reset();
    };

    async function apiJSON(url, init) {
        const response = await fetch(url, {
            headers: { 'Content-Type': 'application/json' },
            ...init,
        });
        const payload = await response.json().catch(() => ({}));
        if (!response.ok) {
            throw new Error(payload.error || 'No se pudo completar la operación.');
        }
        return payload;
    }

    function renderSummary(summary) {
        const reserved = summary?.totalReservedByCurrency || {};
        const cards = Object.keys(reserved).length === 0
            ? '<div class="settings-helper-text">Todavía no tenés ahorros reservados.</div>'
            : Object.entries(reserved)
                .map(([currency, amount]) => `<article class="savings-summary-card"><h3>Ahorro reservado ${currency.toUpperCase()}</h3><strong>${fmtAmount(amount, currency)}</strong></article>`)
                .join('');
        savingsSummaryEl.innerHTML = cards;

        const secondary = summary?.secondaryCurrencyFunds || {};
        if (Object.keys(secondary).length === 0) {
            secondaryCurrencyBoxEl.innerHTML = '<h3>Monedas secundarias</h3><p>No hay tenencias secundarias positivas por ahora.</p>';
        } else {
            secondaryCurrencyBoxEl.innerHTML = `
                <h3>Monedas secundarias (consulta)</h3>
                <p>Estas tenencias externas al módulo también se muestran como referencia de ahorro potencial.</p>
                <ul>${Object.entries(secondary).map(([currency, amount]) => `<li><strong>${currency.toUpperCase()}</strong>: ${fmtAmount(amount, currency)}</li>`).join('')}</ul>
            `;
        }
    }

    function askAmount(actionLabel) {
        const value = window.prompt(`${actionLabel}: ingresá monto`);
        if (!value) return null;
        const parsed = Number(value);
        if (!Number.isFinite(parsed) || parsed <= 0) return null;
        return parsed;
    }

    async function registerAllocation(goalId, action) {
        const amount = askAmount(action === 'contribute' ? 'Aportar' : 'Retirar');
        if (!amount) return;
        await apiJSON(`/savings/goals/${goalId}/${action}`, {
            method: 'POST',
            body: JSON.stringify({ amount }),
        });
        await refresh();
    }

    function renderGoals(summary) {
        const goals = summary?.goals || [];
        if (goals.length === 0) {
            savingsGoalsEl.innerHTML = '<article class="form-container"><h3>Sin metas aún</h3><p>Creá tu primer objetivo de ahorro.</p></article>';
            return;
        }
        savingsGoalsEl.innerHTML = goals.map((item) => {
            const goal = item.goal || {};
            const percentage = Math.round((item.progressRatio || 0) * 100);
            return `
                <article class="savings-goal-card" data-goal-id="${goal.id}">
                    <header>
                        <h3>${goal.name || 'Meta'}</h3>
                        <span class="settings-reports-badge">${(goal.status || 'active').toUpperCase()}</span>
                    </header>
                    <p>${fmtAmount(item.reservedAmount, goal.currency)} de ${fmtAmount(goal.targetAmount, goal.currency)}</p>
                    <div class="savings-progress"><span style="width:${Math.max(0, Math.min(100, percentage))}%"></span></div>
                    <p>Falta: <strong>${fmtAmount(item.remainingAmount, goal.currency)}</strong></p>
                    ${item.monthlySuggestion > 0 ? `<p>Sugerencia mensual: <strong>${fmtAmount(item.monthlySuggestion, goal.currency)}</strong></p>` : ''}
                    <div class="savings-goal-actions">
                        <button class="nav-button button-primary" data-action="contribute">Aportar</button>
                        <button class="nav-button button-secondary" data-action="withdraw">Retirar</button>
                    </div>
                </article>
            `;
        }).join('');

        savingsGoalsEl.querySelectorAll('.savings-goal-card button[data-action]').forEach((button) => {
            button.addEventListener('click', async () => {
                const card = button.closest('.savings-goal-card');
                const goalId = card?.getAttribute('data-goal-id');
                const action = button.getAttribute('data-action');
                if (!goalId || !action) return;
                try {
                    await registerAllocation(goalId, action);
                } catch (error) {
                    window.alert(error.message || 'No se pudo registrar el movimiento.');
                }
            });
        });
    }

    async function refresh() {
        const summary = await apiJSON('/savings/summary', { method: 'GET' });
        renderSummary(summary);
        renderGoals(summary);
    }

    goalForm.addEventListener('submit', async (event) => {
        event.preventDefault();
        const payload = {
            name: document.getElementById('goalName').value,
            targetAmount: Number(document.getElementById('goalTarget').value),
            currency: document.getElementById('goalCurrency').value,
            targetDate: document.getElementById('goalDate').value || '',
        };
        try {
            await apiJSON('/savings/goals', {
                method: 'POST',
                body: JSON.stringify(payload),
            });
            hideGoalModal();
            await refresh();
        } catch (error) {
            goalMessage.textContent = error.message || 'No se pudo guardar la meta.';
            goalMessage.className = 'form-message error';
        }
    });

    if (newGoalBtn) newGoalBtn.addEventListener('click', showGoalModal);
    if (closeModalBtn) closeModalBtn.addEventListener('click', hideGoalModal);
    if (cancelGoalBtn) cancelGoalBtn.addEventListener('click', hideGoalModal);
    modal?.querySelector('[data-goal-close]')?.addEventListener('click', hideGoalModal);

    const boot = async () => {
        try {
            if (typeof checkAuthStatus === 'function') {
                const user = await checkAuthStatus();
                if (!user) return;
            }
            await refresh();
        } catch (error) {
            savingsGoalsEl.innerHTML = `<article class="form-container"><h3>Error</h3><p>${error.message || 'No se pudo cargar Ahorros.'}</p></article>`;
        }
    };

    document.addEventListener('DOMContentLoaded', () => {
        if (typeof initializeMobileNavMenu === 'function') {
            initializeMobileNavMenu();
        }
        boot();
    });
})();
