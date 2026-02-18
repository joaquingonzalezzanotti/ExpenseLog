const colorPalette = [
    '#FF6B6B', '#4ECDC4', '#45B7D1', '#96CEB4', 
    '#FFBE0B', '#FF006E', '#8338EC', '#3A86FF', 
    '#FB5607', '#38B000', '#9B5DE5', '#F15BB5'
];
const currencyBehaviors = {
    ars: {symbol: "$", useComma: true, useDecimals: true, useSpace: false, right: false},
    usd: {symbol: "$", useComma: false, useDecimals: true, useSpace: false, right: false},
    eur: {symbol: "EUR", useComma: true, useDecimals: true, useSpace: false, right: false},
};

function resolveFlow(exp) {
    if (!exp) return 'expense';
    const flow = (exp.flow || '').toLowerCase();
    if (flow) return flow;
    return exp.amount > 0 ? 'income' : 'expense';
}

let authChecked = false;
let currentUser = null;
let pendingAuthErrorMessage = null;
let authCheckPromise = null;
const NOTIFICATION_FETCH_CURRENCY = 'ars';
const NOTIFICATION_FETCH_DAYS = 7;
const NOTIFICATION_REFRESH_MS = 45000;
const NOTIFICATION_SEEN_STORAGE_KEY = 'expenselog_notification_seen_v1';
const NOTIFICATION_DISMISS_STORAGE_KEY = 'expenselog_liquidity_dismiss_v1';
let notificationCenterBound = false;
let notificationCenterTimer = null;
let latestNotificationPayload = null;
let currentVisibleNotificationItems = [];
let notificationCenterDisabled = false;
let notificationCenterFetchWarned = false;

function setAuthPending(pending) {
    if (!document.body || !document.getElementById('authOverlay')) return;
    document.body.classList.toggle('auth-pending', !!pending);
}

setAuthPending(true);

function showAuthMessageFromURL() {
    const overlay = document.getElementById('authOverlay');
    if (!overlay) return;
    const params = new URLSearchParams(window.location.search);
    const authError = params.get('auth_error');
    if (!authError) return;
    pendingAuthErrorMessage = authError;
    params.delete('auth_error');
    const cleaned = params.toString();
    const nextURL = cleaned ? `${window.location.pathname}?${cleaned}` : window.location.pathname;
    window.history.replaceState({}, '', nextURL);
}

async function checkAuthStatus() {
    if (authCheckPromise) return authCheckPromise;

    const overlay = document.getElementById('authOverlay');
    if (!overlay) {
        setAuthPending(false);
        return null;
    }

    authCheckPromise = (async () => {
        let user = null;
        try {
            const res = await fetch('/auth/me', { cache: 'no-store' });
            if (res.ok) {
                user = await res.json();
                currentUser = user;
                updateUserBadge(user);
                hideAuthOverlay();
                ensureNotificationCenter();
                authChecked = true;
                setAuthPending(false);
                return user;
            }
            if (res.status === 401) {
                currentUser = null;
                updateUserBadge(null);
                teardownNotificationCenter();
                if (pendingAuthErrorMessage) {
                    showAuthOverlay(pendingAuthErrorMessage, 'error');
                    pendingAuthErrorMessage = null;
                } else {
                    showAuthOverlay();
                }
                authChecked = true;
                setAuthPending(false);
                return null;
            }
            showAuthOverlay('No se pudo validar la sesion', 'error');
            teardownNotificationCenter();
        } catch (error) {
            console.error('Auth check failed:', error);
            showAuthOverlay('No se pudo validar la sesion', 'error');
            teardownNotificationCenter();
        }
        authChecked = true;
        setAuthPending(false);
        return null;
    })().finally(() => {
        authCheckPromise = null;
    });

    return authCheckPromise;
}

function guardAppInit(initFn) {
    return async () => {
        const user = await checkAuthStatus();
        if (!user) return;
        if (typeof initFn === 'function') {
            await initFn();
        }
    };
}

function showAuthOverlay(message, type) {
    const overlay = document.getElementById('authOverlay');
    if (!overlay) return;
    setAuthPending(false);
    overlay.classList.remove('hidden');
    overlay.setAttribute('aria-hidden', 'false');
    document.body.classList.add('auth-locked');
    const msg = document.getElementById('authMessage');
    if (msg) {
        msg.textContent = message || '';
        if (!message) {
            msg.className = 'form-message';
        } else if (type === 'success') {
            msg.className = 'form-message success';
        } else {
            msg.className = 'form-message error';
        }
    }
}

function hideAuthOverlay() {
    const overlay = document.getElementById('authOverlay');
    if (!overlay) return;
    overlay.classList.add('hidden');
    overlay.setAttribute('aria-hidden', 'true');
    document.body.classList.remove('auth-locked');
}

function userInitialsFromUser(user) {
    if (!user || typeof user !== 'object') return '';
    const displayName = (user.name || user.displayName || '').trim();
    if (displayName) {
        const parts = displayName.split(/\s+/).filter(Boolean);
        const initials = parts.slice(0, 2).map((part) => part.charAt(0).toUpperCase()).join('');
        if (initials) return initials;
    }
    const normalized = (user.email || '').trim().toLowerCase();
    if (!normalized) return '';
    const localPart = normalized.split('@')[0] || '';
    const chars = localPart.replace(/[^a-z0-9]/g, '');
    if (!chars) return '';
    return chars.slice(0, 2).toUpperCase();
}

function updateUserBadge(user) {
    const badge = document.getElementById('userEmailBadge');
    if (!badge) return;
    if (user && (user.email || user.name || user.displayName)) {
        badge.textContent = userInitialsFromUser(user);
        badge.setAttribute('aria-label', 'Usuario conectado');
        badge.style.display = 'inline-flex';
    } else {
        badge.textContent = '';
        badge.removeAttribute('aria-label');
        badge.style.display = 'none';
    }
}

function getNotificationCenterDOM() {
    const button = document.getElementById('notificationCenterButton');
    const badge = document.getElementById('notificationCenterBadge');
    const panel = document.getElementById('notificationCenterPanel');
    const list = document.getElementById('notificationCenterList');
    const icon = button ? button.querySelector('i') : null;
    const slot = button ? button.closest('.nav-right-slot') : null;
    return { button, badge, panel, list, icon, slot };
}

function readJSONStorage(key, fallback) {
    try {
        const raw = localStorage.getItem(key);
        if (!raw) return fallback;
        const parsed = JSON.parse(raw);
        return parsed ?? fallback;
    } catch (error) {
        return fallback;
    }
}

function writeJSONStorage(key, value) {
    try {
        localStorage.setItem(key, JSON.stringify(value));
    } catch (error) {
        // ignore storage write failures
    }
}

function readSeenNotificationKeys() {
    const raw = readJSONStorage(NOTIFICATION_SEEN_STORAGE_KEY, []);
    if (!Array.isArray(raw)) return new Set();
    return new Set(raw.filter(item => typeof item === 'string' && item));
}

function saveSeenNotificationKeys(keys) {
    writeJSONStorage(NOTIFICATION_SEEN_STORAGE_KEY, Array.from(keys));
}

function readNotificationDismissMap() {
    const value = readJSONStorage(NOTIFICATION_DISMISS_STORAGE_KEY, {});
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
        return {};
    }
    return value;
}

function writeNotificationDismissMap(map) {
    writeJSONStorage(NOTIFICATION_DISMISS_STORAGE_KEY, map);
}

function buildNotificationKey(item) {
    if (typeof ExpenseLogAlertUI !== 'undefined' && typeof ExpenseLogAlertUI.buildLiquidityAlertKey === 'function') {
        return ExpenseLogAlertUI.buildLiquidityAlertKey(item);
    }
    const recurringId = String(item?.recurringId || 'sin-recurring');
    const dueDate = String(item?.dueDate || '');
    const kind = String(item?.kind || 'preview_7d');
    return `${recurringId}|${dueDate}|${kind}`;
}

function shouldHideDismissedNotification(item, dismissMap, reappearDays) {
    if (typeof ExpenseLogAlertUI !== 'undefined' && typeof ExpenseLogAlertUI.shouldHideDismissedAlert === 'function') {
        return ExpenseLogAlertUI.shouldHideDismissedAlert(item, dismissMap, reappearDays);
    }
    const key = buildNotificationKey(item);
    const entry = dismissMap[key];
    if (!entry) return false;
    const daysUntil = Number(item?.daysUntil || 0);
    const dismissedDaysUntil = typeof entry === 'object'
        ? Number(entry.daysUntil ?? Number.POSITIVE_INFINITY)
        : Number.POSITIVE_INFINITY;
    const is24hWindow = daysUntil <= reappearDays;
    const wasDismissedBeforeWindow = dismissedDaysUntil > reappearDays;
    if (is24hWindow && wasDismissedBeforeWindow) {
        return false;
    }
    return true;
}

function getNotificationLead(item, criticalDays) {
    if (item?.kind === 'due') return 'Vencimiento';
    if (item?.kind === 'risk_4d') return `Riesgo en ${criticalDays} dias`;
    if (item?.kind === 'monitor_4d') return `Seguimiento ${criticalDays} dias`;
    return 'Aviso 7 dias';
}

function formatNotificationDueDate(isoDate) {
    if (typeof getDateInputValueFromISO === 'function') {
        const ymd = getDateInputValueFromISO(isoDate);
        if (!ymd) return '-';
        const [year, month, day] = ymd.split('-');
        return `${day}/${month}/${year}`;
    }
    const date = new Date(isoDate);
    if (Number.isNaN(date.getTime())) return '-';
    return date.toLocaleDateString('es-AR');
}

function formatNotificationDays(daysUntil) {
    if (daysUntil < 0) {
        const ago = Math.abs(daysUntil);
        return ago === 1 ? 'hace 1 dia' : `hace ${ago} dias`;
    }
    if (daysUntil === 0) return 'hoy';
    if (daysUntil === 1) return 'en 1 dia';
    return `en ${daysUntil} dias`;
}

function formatNotificationAmount(amount, currencyCode) {
    const code = (currencyCode || 'ars').toLowerCase();
    const behavior = currencyBehaviors[code] || currencyBehaviors.ars;
    const isNegative = amount < 0;
    const absAmount = Math.abs(amount);
    const formatted = new Intl.NumberFormat(behavior.useComma ? 'de-DE' : 'en-US', {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
    }).format(absAmount);
    const result = behavior.right
        ? `${formatted}${behavior.useSpace ? ' ' : ''}${behavior.symbol}`
        : `${behavior.symbol}${behavior.useSpace ? ' ' : ''}${formatted}`;
    return isNegative ? `-${result}` : result;
}

function getVisibleNotificationItems(payload) {
    const alerts = Array.isArray(payload?.alerts) ? payload.alerts : [];
    const dismissMap = readNotificationDismissMap();
    const reappearDays = Number(payload?.reappearDays || 1);
    return alerts.filter((item) => !shouldHideDismissedNotification(item, dismissMap, reappearDays));
}

function markCurrentNotificationsAsSeen() {
    if (!currentVisibleNotificationItems.length) return;
    const seenKeys = readSeenNotificationKeys();
    currentVisibleNotificationItems.forEach((item) => {
        seenKeys.add(buildNotificationKey(item));
    });
    saveSeenNotificationKeys(seenKeys);
}

function closeNotificationPanel() {
    const dom = getNotificationCenterDOM();
    if (!dom.panel || !dom.button) return;
    dom.panel.hidden = true;
    dom.button.setAttribute('aria-expanded', 'false');
}

async function openNotificationPanel() {
    const dom = getNotificationCenterDOM();
    if (!dom.panel || !dom.button) return;
    await refreshNotificationCenter();
    dom.panel.hidden = false;
    dom.button.setAttribute('aria-expanded', 'true');
    markCurrentNotificationsAsSeen();
    renderNotificationCenter(latestNotificationPayload);
}

function dismissNotification(itemKey) {
    const selected = currentVisibleNotificationItems.find((item) => buildNotificationKey(item) === itemKey);
    if (!selected) return;
    const dismissMap = readNotificationDismissMap();
    dismissMap[itemKey] = {
        dismissedAt: Date.now(),
        daysUntil: Number(selected?.daysUntil || 0),
    };
    writeNotificationDismissMap(dismissMap);
    renderNotificationCenter(latestNotificationPayload);
}

function renderNotificationCenter(payload) {
    const dom = getNotificationCenterDOM();
    if (!dom.button || !dom.list || !dom.badge) return;

    const visibleItems = getVisibleNotificationItems(payload);
    currentVisibleNotificationItems = visibleItems;
    const topItems = visibleItems.slice(0, 3);
    const hasCritical = visibleItems.some((item) => item?.severity === 'critical');
    const totalCount = visibleItems.length;
    const seenKeys = readSeenNotificationKeys();
    const unseenCount = visibleItems.filter((item) => !seenKeys.has(buildNotificationKey(item))).length;

    if (dom.icon) {
        dom.icon.className = hasCritical ? 'fa-solid fa-triangle-exclamation' : 'fa-regular fa-bell';
    }
    dom.button.classList.toggle('has-alert', hasCritical);

    if (!hasCritical && totalCount > 0 && unseenCount > 0) {
        dom.badge.textContent = String(Math.min(unseenCount, 99));
        dom.badge.hidden = false;
    } else {
        dom.badge.textContent = '';
        dom.badge.hidden = true;
    }

    if (topItems.length === 0) {
        dom.list.innerHTML = '<div class="notification-empty">Sin notificaciones pendientes.</div>';
        return;
    }

    const criticalDays = Number(payload?.criticalDays || 4);
    const currency = String(payload?.currency || NOTIFICATION_FETCH_CURRENCY).toLowerCase();
    dom.list.innerHTML = topItems.map((item) => {
        const itemKey = buildNotificationKey(item);
        const isCritical = item?.severity === 'critical';
        const requiredAmount = Number(item?.requiredAmount || 0);
        const projected = Number(item?.balanceAfter || 0);
        const shortfall = Number(item?.shortfall || 0);
        return `
            <article class="notification-item ${isCritical ? 'is-critical' : ''}" data-item-key="${itemKey}">
                <div class="notification-item-head">
                    <div class="notification-item-title">${escapeHTML(item?.name || 'Recurrente')}</div>
                    <span class="notification-item-badge ${isCritical ? 'critical' : ''}">${isCritical ? 'Alerta' : 'Info'}</span>
                </div>
                <div class="notification-item-summary">${getNotificationLead(item, criticalDays)}: ${formatNotificationDays(Number(item?.daysUntil || 0))} (${formatNotificationDueDate(item?.dueDate)})</div>
                <div class="notification-item-details">
                    <div>Importe: ${formatNotificationAmount(requiredAmount, currency)}</div>
                    <div>Saldo proyectado: ${formatNotificationAmount(projected, currency)}</div>
                    ${shortfall > 0 ? `<div>Faltante estimado: ${formatNotificationAmount(shortfall, currency)}</div>` : ''}
                </div>
                <div class="notification-item-actions">
                    <button type="button" class="notification-item-btn" data-notif-action="toggle" data-notif-key="${itemKey}">Ver completa</button>
                    <button type="button" class="notification-item-btn danger" data-notif-action="dismiss" data-notif-key="${itemKey}">Borrar</button>
                </div>
            </article>
        `;
    }).join('') + (visibleItems.length > 3 ? `<div class="notification-list-more">Mostrando ultimas 3 de ${visibleItems.length}</div>` : '');
}

function bindNotificationCenter() {
    if (notificationCenterBound) return;
    const dom = getNotificationCenterDOM();
    if (!dom.button || !dom.panel || !dom.list) return;

    dom.button.addEventListener('click', (event) => {
        event.stopPropagation();
        if (dom.panel.hidden) {
            openNotificationPanel();
        } else {
            closeNotificationPanel();
        }
    });

    dom.list.addEventListener('click', (event) => {
        const actionButton = event.target.closest('[data-notif-action]');
        if (!actionButton) return;
        const action = actionButton.dataset.notifAction;
        const itemKey = actionButton.dataset.notifKey;
        if (!itemKey) return;
        if (action === 'dismiss') {
            dismissNotification(itemKey);
            return;
        }
        if (action === 'toggle') {
            const itemNode = actionButton.closest('.notification-item');
            if (!itemNode) return;
            const opened = itemNode.classList.toggle('is-open');
            actionButton.textContent = opened ? 'Ver menos' : 'Ver completa';
        }
    });

    document.addEventListener('click', (event) => {
        if (dom.panel.hidden) return;
        const insideButton = dom.button.contains(event.target);
        const insidePanel = dom.panel.contains(event.target);
        if (!insideButton && !insidePanel) {
            closeNotificationPanel();
        }
    });

    notificationCenterBound = true;
}

async function refreshNotificationCenter() {
    const dom = getNotificationCenterDOM();
    if (!dom.button || !currentUser || notificationCenterDisabled) return;
    try {
        const response = await fetch(`/alerts/liquidity?currency=${NOTIFICATION_FETCH_CURRENCY}&days=${NOTIFICATION_FETCH_DAYS}`, { cache: 'no-store' });
        if (response.status === 404) {
            notificationCenterDisabled = true;
            if (notificationCenterTimer) {
                window.clearInterval(notificationCenterTimer);
                notificationCenterTimer = null;
            }
            latestNotificationPayload = { alerts: [] };
            renderNotificationCenter(latestNotificationPayload);
            if (dom.slot) dom.slot.style.display = 'none';
            return;
        }
        if (!response.ok) throw new Error(`status ${response.status}`);
        latestNotificationPayload = await response.json();
        notificationCenterFetchWarned = false;
        renderNotificationCenter(latestNotificationPayload);
    } catch (error) {
        if (!notificationCenterFetchWarned) {
            console.warn('No se pudo refrescar el centro de notificaciones:', error);
            notificationCenterFetchWarned = true;
        }
        latestNotificationPayload = { alerts: [] };
        renderNotificationCenter(latestNotificationPayload);
    }
}

function ensureNotificationCenter() {
    const dom = getNotificationCenterDOM();
    if (!dom.button || notificationCenterDisabled) return;
    if (dom.slot) {
        dom.slot.style.display = 'inline-flex';
    }
    bindNotificationCenter();
    refreshNotificationCenter();
    if (!notificationCenterTimer) {
        notificationCenterTimer = window.setInterval(refreshNotificationCenter, NOTIFICATION_REFRESH_MS);
    }
}

function teardownNotificationCenter() {
    const dom = getNotificationCenterDOM();
    if (notificationCenterTimer) {
        window.clearInterval(notificationCenterTimer);
        notificationCenterTimer = null;
    }
    latestNotificationPayload = null;
    currentVisibleNotificationItems = [];
    if (dom.badge) {
        dom.badge.hidden = true;
        dom.badge.textContent = '';
    }
    if (dom.panel) {
        dom.panel.hidden = true;
    }
    if (dom.slot) {
        dom.slot.style.display = 'none';
    }
}

async function expenseLogLogout() {
    try {
        await fetch('/auth/logout', { method: 'POST' });
    } catch (error) {
        console.error('Logout failed:', error);
    } finally {
        teardownNotificationCenter();
        showAuthOverlay();
        window.location.reload();
    }
}

window.expenseLogLogout = expenseLogLogout;

function setAuthTab(tab) {
    const tabs = document.querySelectorAll('.auth-tab');
    const forms = document.querySelectorAll('.auth-form');
    tabs.forEach(btn => {
        btn.classList.toggle('active', btn.dataset.authTab === tab);
    });
    forms.forEach(form => {
        form.classList.toggle('active', form.dataset.authForm === tab);
    });
}

function setupAuthUI() {
    const overlay = document.getElementById('authOverlay');
    if (!overlay || overlay.dataset.bound === 'true') return;
    overlay.dataset.bound = 'true';

    const tabs = document.querySelectorAll('.auth-tab');
    tabs.forEach(tab => {
        tab.addEventListener('click', () => setAuthTab(tab.dataset.authTab));
    });
    const links = document.querySelectorAll('[data-auth-link]');
    links.forEach(link => {
        link.addEventListener('click', () => setAuthTab(link.dataset.authLink));
    });

    const loginForm = document.getElementById('authLoginForm');
    if (loginForm) {
        loginForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const email = document.getElementById('authLoginEmail').value.trim();
            const password = document.getElementById('authLoginPassword').value;
            const remember = !!document.getElementById('authLoginRemember')?.checked;
            try {
                const response = await fetch('/auth/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ email, password, remember }),
                });
                if (!response.ok) {
                    const error = await response.json().catch(() => ({}));
                    showAuthOverlay(error.error || 'No se pudo iniciar sesion', 'error');
                    return;
                }
                hideAuthOverlay();
                window.location.reload();
            } catch (error) {
                console.error('Login failed:', error);
                showAuthOverlay('No se pudo iniciar sesion', 'error');
            }
        });
    }

    const registerForm = document.getElementById('authRegisterForm');
    if (registerForm) {
        registerForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const email = document.getElementById('authRegisterEmail').value.trim();
            const password = document.getElementById('authRegisterPassword').value;
            const remember = !!document.getElementById('authRegisterRemember')?.checked;
            try {
                const response = await fetch('/auth/register', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ email, password, remember }),
                });
                if (!response.ok) {
                    const error = await response.json().catch(() => ({}));
                    showAuthOverlay(error.error || 'No se pudo registrar', 'error');
                    return;
                }
                registerForm.reset();
                showAuthOverlay('Te enviamos un email para verificar tu cuenta. Revisa tu correo.', 'success');
                setAuthTab('login');
            } catch (error) {
                console.error('Register failed:', error);
                showAuthOverlay('No se pudo registrar', 'error');
            }
        });
    }

    const resetSendButton = document.getElementById('authSendResetCode');
    if (resetSendButton) {
        resetSendButton.addEventListener('click', async () => {
            const email = document.getElementById('authResetEmail')?.value.trim();
            if (!email) {
                showAuthOverlay('Ingresa un email valido', 'error');
                return;
            }
            try {
                const response = await fetch('/auth/reset/request', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ email }),
                });
                if (!response.ok) {
                    const error = await response.json().catch(() => ({}));
                    showAuthOverlay(error.error || 'No se pudo enviar el codigo', 'error');
                    return;
                }
                showAuthOverlay('Te enviamos un codigo al email', 'success');
            } catch (error) {
                console.error('Reset request failed:', error);
                showAuthOverlay('No se pudo enviar el codigo', 'error');
            }
        });
    }

    const resetForm = document.getElementById('authResetForm');
    if (resetForm) {
        resetForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const email = document.getElementById('authResetEmail')?.value.trim();
            const code = document.getElementById('authResetCode')?.value.trim();
            const password = document.getElementById('authResetPassword')?.value || '';
            if (!email || !code || !password) {
                showAuthOverlay('Completa todos los campos', 'error');
                return;
            }
            try {
                const response = await fetch('/auth/reset/confirm', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ email, code, password }),
                });
                if (!response.ok) {
                    const error = await response.json().catch(() => ({}));
                    showAuthOverlay(error.error || 'No se pudo actualizar la contraseña', 'error');
                    return;
                }
                showAuthOverlay('Contraseña actualizada. Ya podes ingresar.', 'success');
                setAuthTab('login');
            } catch (error) {
                console.error('Reset confirm failed:', error);
                showAuthOverlay('No se pudo actualizar la contraseña', 'error');
            }
        });
    }

    const logoutButton = document.getElementById('logoutButton');
    if (logoutButton) {
        logoutButton.addEventListener('click', expenseLogLogout);
    }

    setAuthTab('login');
}

document.addEventListener('DOMContentLoaded', () => {
    setupAuthUI();
    showAuthMessageFromURL();
    if (!authChecked) {
        checkAuthStatus();
    }
});

window.addEventListener('focus', () => {
    if (!document.getElementById('authOverlay')) return;
    checkAuthStatus();
});

function formatCurrency(amount) {
    const behavior = currencyBehaviors[currentCurrency] || {
        symbol: "$",
        useComma: false,
        useDecimals: true,
        useSpace: false,
        right: false,
    };
    const isNegative = amount < 0;
    const absAmount = Math.abs(amount);
    const options = {
        minimumFractionDigits: behavior.useDecimals ? 2 : 0,
        maximumFractionDigits: behavior.useDecimals ? 2 : 0,
    };
    let formattedAmount = new Intl.NumberFormat(behavior.useComma ? "de-DE" : "en-US",options).format(absAmount);
    let result = behavior.right
        ? `${formattedAmount}${behavior.useSpace ? " " : ""}${behavior.symbol}`
        : `${behavior.symbol}${behavior.useSpace ? " " : ""}${formattedAmount}`;
    return isNegative ? `-${result}` : result;
}

function getUserTimeZone() {
    return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

function formatMonth(date) {
    const formatted = date.toLocaleDateString('es-AR', {
        year: 'numeric',
        month: 'long',
        timeZone: getUserTimeZone()
    });
    // Capitaliza la primera letra para mostrar el mes en mayuscula inicial.
    return formatted.charAt(0).toUpperCase() + formatted.slice(1);
}

function getISODateWithLocalTime(dateInput) {
    const [year, month, day] = dateInput.split('-').map(Number);
    const now = new Date();
    const hours = now.getHours();
    const minutes = now.getMinutes();
    const seconds = now.getSeconds();
    const localDateTime = new Date(year, month - 1, day, hours, minutes, seconds);
    return localDateTime.toISOString();
}

// Stores date-only fields (like recurring start date) at local noon to avoid timezone day-shifts.
function getISODateWithLocalNoon(dateInput) {
    const [year, month, day] = dateInput.split('-').map(Number);
    const localNoon = new Date(year, month - 1, day, 12, 0, 0, 0);
    return localNoon.toISOString();
}

// Reads ISO date fields as calendar dates (UTC components) for stable YYYY-MM-DD inputs.
function getDateInputValueFromISO(isoDateString) {
    const date = new Date(isoDateString);
    if (Number.isNaN(date.getTime())) return '';
    const year = date.getUTCFullYear();
    const month = String(date.getUTCMonth() + 1).padStart(2, '0');
    const day = String(date.getUTCDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
}

function formatDateFromUTC(utcDateString) {
    const date = new Date(utcDateString);
    return date.toLocaleDateString('es-AR', {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        timeZoneName: 'short'
    });
}

function updateMonthDisplay() {
    const currentMonthEl = document.getElementById('currentMonth');
    if (currentMonthEl) {
        currentMonthEl.textContent = formatMonth(currentDate);
    }
}

function getMonthBounds(date) {
    const localDate = new Date(date);
    if (startDate === 1) {
        const startLocal = new Date(localDate.getFullYear(), localDate.getMonth(), 1);
        const endLocal = new Date(localDate.getFullYear(), localDate.getMonth() + 1, 0, 23, 59, 59, 999);
        return { start: new Date(startLocal.toISOString()), end: new Date(endLocal.toISOString()) };
    }
    let thisMonthStartDate = startDate;
    let prevMonthStartDate = startDate;

    const currentMonth = localDate.getMonth();
    const currentYear = localDate.getFullYear();
    const daysInCurrentMonth = new Date(currentYear, currentMonth + 1, 0).getDate();
    thisMonthStartDate = Math.min(thisMonthStartDate, daysInCurrentMonth);
    const prevMonth = currentMonth === 0 ? 11 : currentMonth - 1;
    const prevYear = currentMonth === 0 ? currentYear - 1 : currentYear;
    const daysInPrevMonth = new Date(prevYear, prevMonth + 1, 0).getDate();
    prevMonthStartDate = Math.min(prevMonthStartDate, daysInPrevMonth);

    if (localDate.getDate() < thisMonthStartDate) {
        const startLocal = new Date(prevYear, prevMonth, prevMonthStartDate);
        const endLocal = new Date(currentYear, currentMonth, thisMonthStartDate - 1, 23, 59, 59, 999);
        return { start: new Date(startLocal.toISOString()), end: new Date(endLocal.toISOString()) };
    } else {
        const nextMonth = currentMonth === 11 ? 0 : currentMonth + 1;
        const nextYear = currentMonth === 11 ? currentYear + 1 : currentYear;
        const daysInNextMonth = new Date(nextYear, nextMonth + 1, 0).getDate();
        let nextMonthStartDate = Math.min(startDate, daysInNextMonth);
        const startLocal = new Date(currentYear, currentMonth, thisMonthStartDate);
        const endLocal = new Date(nextYear, nextMonth, nextMonthStartDate - 1, 23, 59, 59, 999);
        return { start: new Date(startLocal.toISOString()), end: new Date(endLocal.toISOString()) };
    }
}

function getComparableExpenseDate(exp) {
    const rawDate = new Date(exp?.date);
    if (!exp?.recurringID) return rawDate;
    const ymd = getDateInputValueFromISO(exp.date);
    if (!ymd) return rawDate;
    const [year, month, day] = ymd.split('-').map(Number);
    return new Date(year, month - 1, day, 12, 0, 0, 0);
}

function getMonthExpenses(expenses) {
    const { start, end } = getMonthBounds(currentDate);
    return expenses.filter(exp => {
        const expDate = getComparableExpenseDate(exp);
        return expDate >= start && expDate <= end;
    }).sort((a, b) => getComparableExpenseDate(b) - getComparableExpenseDate(a));
}

function escapeHTML(str) {
    if (typeof str !== 'string') return str;
    return str.replace(/[&<>'"]/g,
        tag => ({
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            "'": '&#39;',
            '"': '&quot;'
        }[tag] || tag)
    );
}

function escapeRegex(str) {
    return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function showToast(message, type) {
    const container = document.getElementById('toastContainer');
    if (!container) return;
    const toast = document.createElement('div');
    toast.className = `toast ${type || ''}`.trim();
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(() => {
        toast.remove();
    }, 3000);
}
