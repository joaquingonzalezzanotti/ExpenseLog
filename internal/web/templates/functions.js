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
                authChecked = true;
                setAuthPending(false);
                return user;
            }
            if (res.status === 401) {
                currentUser = null;
                updateUserBadge(null);
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
        } catch (error) {
            console.error('Auth check failed:', error);
            showAuthOverlay('No se pudo validar la sesion', 'error');
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
        logoutButton.addEventListener('click', async () => {
            try {
                await fetch('/auth/logout', { method: 'POST' });
            } catch (error) {
                console.error('Logout failed:', error);
            } finally {
                showAuthOverlay();
                window.location.reload();
            }
        });
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
