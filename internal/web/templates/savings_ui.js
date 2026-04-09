(function() {
    const savingsSummaryEl = document.getElementById('savingsSummary');
    const secondaryCurrencyBoxEl = document.getElementById('secondaryCurrencyBox');
    const savingsGoalsEl = document.getElementById('savingsGoals');
    const goalsCountEl = document.getElementById('savingsGoalsCount');
    const newGoalBtn = document.getElementById('newGoalBtn');
    const modal = document.getElementById('savingsGoalModal');
    const closeModalBtn = document.getElementById('closeGoalModalBtn');
    const cancelGoalBtn = document.getElementById('cancelGoalBtn');
    const goalForm = document.getElementById('savingsGoalForm');
    const goalMessage = document.getElementById('savingsGoalMessage');
    const movementModal = document.getElementById('savingsMovementModal');
    const closeMovementModalBtn = document.getElementById('closeMovementModalBtn');
    const cancelMovementBtn = document.getElementById('cancelMovementBtn');
    const movementForm = document.getElementById('savingsMovementForm');
    const movementMessage = document.getElementById('savingsMovementMessage');
    const movementTypeLabel = document.getElementById('movementTypeLabel');
    const movementGoalLabel = document.getElementById('movementGoalLabel');
    const movementGoalIdInput = document.getElementById('movementGoalId');
    const movementActionInput = document.getElementById('movementAction');
    const movementAmountInput = document.getElementById('movementAmount');
    const movementDateInput = document.getElementById('movementDate');
    const movementNoteInput = document.getElementById('movementNote');
    const saveMovementBtn = document.getElementById('saveMovementBtn');

    const statusMeta = {
        active: { label: 'Activa', tone: 'active' },
        completed: { label: 'Completada', tone: 'completed' },
        archived: { label: 'Archivada', tone: 'archived' },
    };

    let movementContext = {
        goalId: '',
        goalName: '',
        action: 'contribute',
    };

    const fmtAmount = (amount, currency) => {
        const normalized = String(currency || 'ars').toLowerCase();
        const locale = normalized === 'usd' ? 'en-US' : 'es-AR';
        return new Intl.NumberFormat(locale, {
            style: 'currency',
            currency: normalized.toUpperCase(),
            maximumFractionDigits: 2,
        }).format(Number(amount || 0));
    };

    const fmtDate = (value) => {
        if (!value) return 'Sin fecha objetivo';
        const date = new Date(value);
        if (Number.isNaN(date.getTime())) return 'Sin fecha objetivo';
        return new Intl.DateTimeFormat('es-AR', { day: '2-digit', month: 'short', year: 'numeric' }).format(date);
    };

    const escapeAttr = (value) => String(value || '')
        .replace(/&/g, '&amp;')
        .replace(/"/g, '&quot;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');

    const showGoalModal = () => {
        if (!modal) return;
        modal.hidden = false;
        modal.setAttribute('aria-hidden', 'false');
    };

    const hideGoalModal = () => {
        if (!modal) return;
        modal.hidden = true;
        modal.setAttribute('aria-hidden', 'true');
        if (goalMessage) {
            goalMessage.textContent = '';
            goalMessage.className = 'form-message';
        }
        goalForm?.reset();
    };

    const showMovementModal = ({ goalId, goalName, action }) => {
        if (!movementModal) return;
        movementContext = {
            goalId: String(goalId || '').trim(),
            goalName: String(goalName || 'Meta'),
            action: action === 'withdraw' ? 'withdraw' : 'contribute',
        };
        if (!movementContext.goalId) return;

        const isWithdraw = movementContext.action === 'withdraw';
        if (movementTypeLabel) {
            movementTypeLabel.textContent = isWithdraw ? 'Retiro' : 'Aporte';
        }
        if (movementGoalLabel) {
            movementGoalLabel.textContent = movementContext.goalName;
        }
        if (movementGoalIdInput) movementGoalIdInput.value = movementContext.goalId;
        if (movementActionInput) movementActionInput.value = movementContext.action;
        if (saveMovementBtn) {
            saveMovementBtn.textContent = isWithdraw ? 'Registrar retiro' : 'Registrar aporte';
        }
        if (movementMessage) {
            movementMessage.textContent = '';
            movementMessage.className = 'form-message';
        }
        movementForm?.reset();
        movementModal.hidden = false;
        movementModal.setAttribute('aria-hidden', 'false');
        movementAmountInput?.focus();
    };

    const hideMovementModal = () => {
        if (!movementModal) return;
        movementModal.hidden = true;
        movementModal.setAttribute('aria-hidden', 'true');
        movementForm?.reset();
        if (movementMessage) {
            movementMessage.textContent = '';
            movementMessage.className = 'form-message';
        }
    };

    async function apiJSON(url, init) {
        const response = await fetch(url, {
            headers: { 'Content-Type': 'application/json' },
            ...init,
        });
        const payload = await response.json().catch(() => ({}));
        if (!response.ok) {
            if (response.status === 401) {
                throw new Error('Sesion expirada. Inicia sesion de nuevo en /app/perfil.');
            }
            throw new Error(payload.error || 'No se pudo completar la operacion.');
        }
        return payload;
    }

    function setGoalsCount(total, active) {
        if (!goalsCountEl) return;
        const totalText = `${total} ${total === 1 ? 'meta' : 'metas'}`;
        const activeText = `${active} activas`;
        goalsCountEl.textContent = `${totalText} · ${activeText}`;
    }

    function normalizeStatus(status, progressRatio) {
        const code = String(status || 'active').trim().toLowerCase();
        if (code === 'completed' || progressRatio >= 1) return statusMeta.completed;
        if (code === 'archived') return statusMeta.archived;
        return statusMeta.active;
    }

    function renderSummary(summary) {
        const goals = Array.isArray(summary?.goals) ? summary.goals : [];
        const reserved = summary?.totalReservedByCurrency || {};

        const activeGoals = goals.filter((entry) => {
            const snapshot = entry || {};
            const goal = snapshot.goal || {};
            const progressRatio = Number(snapshot.progressRatio || 0);
            const normalized = normalizeStatus(goal.status, progressRatio);
            return normalized.tone === 'active';
        }).length;

        const completedGoals = goals.filter((entry) => {
            const snapshot = entry || {};
            const goal = snapshot.goal || {};
            const progressRatio = Number(snapshot.progressRatio || 0);
            const normalized = normalizeStatus(goal.status, progressRatio);
            return normalized.tone === 'completed';
        }).length;

        const totalReservedCards = Object.entries(reserved).map(([currency, amount]) => `
            <article class="savings-summary-card savings-summary-money" data-currency="${currency}">
                <div class="savings-summary-card-top">
                    <span class="savings-summary-icon"><i class="fa-solid fa-piggy-bank" aria-hidden="true"></i></span>
                    <span class="savings-summary-title">Reservado ${currency.toUpperCase()}</span>
                </div>
                <strong>${fmtAmount(amount, currency)}</strong>
                <p>Saldo acumulado en metas de ahorro.</p>
            </article>
        `).join('');

        const reservedFallback = totalReservedCards || `
            <article class="savings-summary-card savings-summary-money">
                <div class="savings-summary-card-top">
                    <span class="savings-summary-icon"><i class="fa-solid fa-piggy-bank" aria-hidden="true"></i></span>
                    <span class="savings-summary-title">Reservado total</span>
                </div>
                <strong>${fmtAmount(0, 'ars')}</strong>
                <p>Cuando registres aportes, veras tu ahorro acumulado aqui.</p>
            </article>
        `;

        savingsSummaryEl.innerHTML = `
            <article class="savings-summary-card savings-summary-kpi">
                <div class="savings-summary-card-top">
                    <span class="savings-summary-icon"><i class="fa-solid fa-bullseye" aria-hidden="true"></i></span>
                    <span class="savings-summary-title">Metas activas</span>
                </div>
                <strong>${activeGoals}</strong>
                <p>${goals.length} metas creadas en total.</p>
            </article>
            <article class="savings-summary-card savings-summary-kpi">
                <div class="savings-summary-card-top">
                    <span class="savings-summary-icon"><i class="fa-solid fa-flag-checkered" aria-hidden="true"></i></span>
                    <span class="savings-summary-title">Metas cumplidas</span>
                </div>
                <strong>${completedGoals}</strong>
                <p>${goals.length > 0 ? Math.round((completedGoals / goals.length) * 100) : 0}% de cumplimiento historico.</p>
            </article>
            ${reservedFallback}
        `;

        setGoalsCount(goals.length, activeGoals);
    }

    function renderSecondaryCurrency(summary) {
        const secondary = summary?.secondaryCurrencyFunds || {};
        const entries = Object.entries(secondary);

        if (entries.length === 0) {
            secondaryCurrencyBoxEl.innerHTML = `
                <div class="savings-secondary-head">
                    <h3>Monedas secundarias</h3>
                    <span class="savings-secondary-badge">Referencia</span>
                </div>
                <p>No hay tenencias secundarias positivas por ahora.</p>
            `;
            return;
        }

        secondaryCurrencyBoxEl.innerHTML = `
            <div class="savings-secondary-head">
                <h3>Monedas secundarias (consulta)</h3>
                <span class="savings-secondary-badge">Referencia</span>
            </div>
            <p>Estas tenencias externas al modulo sirven como reserva potencial para nuevas metas.</p>
            <ul class="savings-secondary-list">
                ${entries.map(([currency, amount]) => `
                    <li>
                        <span>${currency.toUpperCase()}</span>
                        <strong>${fmtAmount(amount, currency)}</strong>
                    </li>
                `).join('')}
            </ul>
        `;
    }

    function parseMovementAmount() {
        const raw = String(movementAmountInput?.value || '').trim().replace(',', '.');
        const parsed = Number(raw);
        if (!Number.isFinite(parsed) || parsed <= 0) return null;
        return parsed;
    }

    function renderAllocations(item, currency) {
        const allocations = Array.isArray(item?.allocations) ? item.allocations.slice(0, 3) : [];
        if (allocations.length === 0) {
            return '<p class="savings-activity-empty">Sin movimientos recientes.</p>';
        }

        return `
            <div class="savings-activity">
                <h4>Actividad reciente</h4>
                <ul>
                    ${allocations.map((movement) => {
                        const kind = String(movement?.type || 'contribution').toLowerCase();
                        const isWithdrawal = kind === 'withdrawal';
                        const label = isWithdrawal ? 'Retiro' : 'Aporte';
                        const sign = isWithdrawal ? '-' : '+';
                        return `
                            <li>
                                <span class="savings-activity-type ${isWithdrawal ? 'is-withdrawal' : 'is-contribution'}">${label}</span>
                                <strong>${sign}${fmtAmount(movement?.amount, currency)}</strong>
                                <small>${fmtDate(movement?.date)}</small>
                            </li>
                        `;
                    }).join('')}
                </ul>
            </div>
        `;
    }

    function renderGoals(summary) {
        const goals = Array.isArray(summary?.goals) ? summary.goals : [];

        if (goals.length === 0) {
            savingsGoalsEl.innerHTML = `
                <article class="savings-empty-state">
                    <div class="savings-empty-visual" aria-hidden="true"><i class="fa-solid fa-piggy-bank"></i></div>
                    <h3>Sin metas todavia</h3>
                    <p>Crea tu primer objetivo y empieza a separar ahorro de gasto diario.</p>
                    <button class="nav-button button-primary" type="button" data-action="new-goal">Crear primera meta</button>
                </article>
            `;
            savingsGoalsEl.querySelector('[data-action="new-goal"]')?.addEventListener('click', showGoalModal);
            return;
        }

        savingsGoalsEl.innerHTML = goals.map((item) => {
            const goal = item?.goal || {};
            const percentage = Math.max(0, Math.min(100, Math.round(Number(item?.progressRatio || 0) * 100)));
            const status = normalizeStatus(goal.status, Number(item?.progressRatio || 0));
            const dueLabel = fmtDate(goal.targetDate);

            return `
                <article class="savings-goal-card" data-goal-id="${goal.id}" data-goal-name="${escapeAttr(goal.name || 'Meta')}">
                    <header class="savings-goal-header">
                        <div>
                            <h3>${goal.name || 'Meta'}</h3>
                            <p class="savings-goal-deadline">Objetivo: ${dueLabel}</p>
                        </div>
                        <span class="savings-goal-status is-${status.tone}">${status.label}</span>
                    </header>

                    <div class="savings-goal-amounts">
                        <strong>${fmtAmount(item?.reservedAmount, goal.currency)}</strong>
                        <span>de ${fmtAmount(goal.targetAmount, goal.currency)}</span>
                    </div>

                    <div class="savings-progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${percentage}" aria-label="Progreso ${goal.name || 'meta'}">
                        <span style="width:${percentage}%"></span>
                    </div>
                    <p class="savings-progress-caption">${percentage}% completado</p>

                    <div class="savings-goal-breakdown">
                        <p>Falta: <strong>${fmtAmount(item?.remainingAmount, goal.currency)}</strong></p>
                        ${item?.monthlySuggestion > 0 ? `<p>Ritmo sugerido: <strong>${fmtAmount(item.monthlySuggestion, goal.currency)}/mes</strong></p>` : '<p>Sin ritmo sugerido por fecha objetivo.</p>'}
                    </div>

                    ${renderAllocations(item, goal.currency)}

                    <div class="savings-goal-actions">
                        <button class="nav-button button-primary" data-action="contribute" type="button"><i class="fa-solid fa-plus" aria-hidden="true"></i> Aportar</button>
                        <button class="nav-button button-secondary" data-action="withdraw" type="button"><i class="fa-solid fa-arrow-up-from-bracket" aria-hidden="true"></i> Retirar</button>
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

                const goalName = card?.getAttribute('data-goal-name') || 'Meta';
                showMovementModal({ goalId, goalName, action });
            });
        });
    }

    function renderLoadingState() {
        savingsSummaryEl.innerHTML = `
            <article class="savings-summary-card savings-summary-loading"></article>
            <article class="savings-summary-card savings-summary-loading"></article>
            <article class="savings-summary-card savings-summary-loading"></article>
        `;
        secondaryCurrencyBoxEl.innerHTML = '<p>Cargando datos...</p>';
        savingsGoalsEl.innerHTML = '<article class="savings-empty-state"><p>Cargando metas...</p></article>';
    }

    async function refresh() {
        const summary = await apiJSON('/savings/summary', { method: 'GET' });
        renderSummary(summary);
        renderSecondaryCurrency(summary);
        renderGoals(summary);
    }

    goalForm?.addEventListener('submit', async (event) => {
        event.preventDefault();
        const payload = {
            name: document.getElementById('goalName')?.value || '',
            targetAmount: Number(document.getElementById('goalTarget')?.value || 0),
            currency: document.getElementById('goalCurrency')?.value || 'ars',
            targetDate: document.getElementById('goalDate')?.value || '',
        };

        try {
            await apiJSON('/savings/goals', {
                method: 'POST',
                body: JSON.stringify(payload),
            });
            hideGoalModal();
            await refresh();
        } catch (error) {
            if (!goalMessage) return;
            goalMessage.textContent = error.message || 'No se pudo guardar la meta.';
            goalMessage.className = 'form-message error';
        }
    });

    movementForm?.addEventListener('submit', async (event) => {
        event.preventDefault();
        const goalId = String(movementGoalIdInput?.value || movementContext.goalId || '').trim();
        const action = String(movementActionInput?.value || movementContext.action || 'contribute').trim().toLowerCase();
        const amount = parseMovementAmount();
        const note = String(movementNoteInput?.value || '').trim();
        const date = String(movementDateInput?.value || '').trim();

        if (!goalId) return;
        if (!amount) {
            if (!movementMessage) return;
            movementMessage.textContent = 'Ingresa un monto valido mayor a cero.';
            movementMessage.className = 'form-message error';
            return;
        }

        try {
            await apiJSON(`/savings/goals/${goalId}/${action}`, {
                method: 'POST',
                body: JSON.stringify({
                    amount,
                    note,
                    date,
                }),
            });
            hideMovementModal();
            await refresh();
        } catch (error) {
            if (!movementMessage) return;
            movementMessage.textContent = error.message || 'No se pudo registrar el movimiento.';
            movementMessage.className = 'form-message error';
        }
    });

    newGoalBtn?.addEventListener('click', showGoalModal);
    closeModalBtn?.addEventListener('click', hideGoalModal);
    cancelGoalBtn?.addEventListener('click', hideGoalModal);
    modal?.querySelector('[data-goal-close]')?.addEventListener('click', hideGoalModal);
    closeMovementModalBtn?.addEventListener('click', hideMovementModal);
    cancelMovementBtn?.addEventListener('click', hideMovementModal);
    movementModal?.querySelector('[data-movement-close]')?.addEventListener('click', hideMovementModal);

    const boot = async () => {
        renderLoadingState();
        try {
            if (typeof checkAuthStatus === 'function') {
                await checkAuthStatus();
            }
            await refresh();
        } catch (error) {
            savingsGoalsEl.innerHTML = `
                <article class="savings-empty-state savings-error-state">
                    <h3>No pudimos cargar tus metas</h3>
                    <p>${error.message || 'Intenta recargar la pagina en unos segundos.'}</p>
                    <button class="nav-button button-primary" type="button" data-action="retry">Reintentar</button>
                </article>
            `;
            savingsGoalsEl.querySelector('[data-action="retry"]')?.addEventListener('click', () => {
                boot();
            });
        }
    };

    document.addEventListener('DOMContentLoaded', () => {
        if (typeof initializeMobileNavMenu === 'function') {
            initializeMobileNavMenu();
        }
        boot();
    });
})();
