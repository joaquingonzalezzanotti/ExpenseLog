const SW_VERSION = '2026-03-10-v7';
const STATIC_CACHE = `expenselog-static-${SW_VERSION}`;
const PAGE_CACHE = `expenselog-pages-${SW_VERSION}`;

const STATIC_ASSETS = [
    '/app/style.css',
    '/app/onboarding.css',
    '/app/functions.js',
    '/app/onboarding_ui.js',
    '/app/alerts_ui.js',
    '/app/cashflow_ui.js',
    '/app/fa.min.css',
    '/app/chart.min.js',
    '/app/manifest.json',
    '/app/favicon.ico',
    '/app/pwa/icon-192.png',
    '/app/pwa/icon-512.png',
];

const APP_ROUTES = [
    '/app',
    '/app/table',
    '/app/table?view=calendar',
    '/app/perfil',
    '/app/categorias',
    '/app/recurrentes',
    '/app/conciliacion',
    '/app/reportes',
    '/app/telegram',
];

function normalizeAppPath(pathname) {
    const cleaned = String(pathname || '').replace(/\/+$/, '') || '/';
    if (cleaned === '/app/index' || cleaned === '/app/index.html') return '/app';
    if (cleaned === '/app/settings') return '/app/perfil';
    return cleaned;
}

function cacheKeyForURL(url) {
    const pathname = normalizeAppPath(url.pathname);
    const search = String(url.search || '');
    if (!search) return pathname;
    return `${pathname}${search}`;
}

async function seedCache(cacheName, urls) {
    const cache = await caches.open(cacheName);
    await Promise.all(urls.map(async (url) => {
        try {
            const response = await fetch(url, { cache: 'no-store' });
            if (response.ok) {
                await cache.put(url, response.clone());
            }
        } catch (_) {
            // Best-effort cache warmup.
        }
    }));
}

self.addEventListener('install', (event) => {
    event.waitUntil((async () => {
        await Promise.all([
            seedCache(STATIC_CACHE, STATIC_ASSETS),
            seedCache(PAGE_CACHE, APP_ROUTES),
        ]);
        await self.skipWaiting();
    })());
});

self.addEventListener('activate', (event) => {
    event.waitUntil((async () => {
        const keys = await caches.keys();
        await Promise.all(keys.map((key) => {
            if (key !== STATIC_CACHE && key !== PAGE_CACHE) {
                return caches.delete(key);
            }
            return Promise.resolve();
        }));
        if (self.registration.navigationPreload) {
            try {
                await self.registration.navigationPreload.enable();
            } catch (_) {
                // Ignore if browser rejects enablement.
            }
        }
        await self.clients.claim();
    })());
});

async function handleNavigate(request, preloadResponsePromise) {
    const url = new URL(request.url);
    const pageCache = await caches.open(PAGE_CACHE);
    const key = cacheKeyForURL(url);
    const cached = await pageCache.match(key) || await pageCache.match(normalizeAppPath(url.pathname));

    const preloadResponse = await (async () => {
        if (!preloadResponsePromise) return null;
        try {
            return await preloadResponsePromise;
        } catch (_) {
            return null;
        }
    })();

    const networkFetch = (async () => {
        if (preloadResponse && preloadResponse.ok) {
            await pageCache.put(key, preloadResponse.clone());
            const normalizedPath = normalizeAppPath(url.pathname);
            if (normalizedPath !== key) {
                await pageCache.put(normalizedPath, preloadResponse.clone());
            }
            return preloadResponse;
        }
        const response = await fetch(request);
        if (response && response.ok) {
            await pageCache.put(key, response.clone());
            const normalizedPath = normalizeAppPath(url.pathname);
            if (normalizedPath !== key) {
                await pageCache.put(normalizedPath, response.clone());
            }
        }
        return response;
    })();

    if (cached) {
        networkFetch.catch(() => {});
        return cached;
    }

    try {
        return await networkFetch;
    } catch (_) {
        return cached || await pageCache.match('/app') || new Response('Sin conexion', {
            status: 503,
            statusText: 'Service Unavailable',
            headers: { 'Content-Type': 'text/plain; charset=utf-8' },
        });
    }
}

function isStaticAssetRequest(request, url) {
    if (request.destination === 'document') return false;
    if (!url.pathname.startsWith('/app/')) return false;
    if (url.pathname === '/app/sw.js') return false;
    return ['script', 'style', 'font', 'image'].includes(request.destination);
}

async function handleStaticAsset(request) {
    const url = new URL(request.url);
    const cache = await caches.open(STATIC_CACHE);
    const cached = await cache.match(url.pathname) || await cache.match(request, { ignoreSearch: true });

    const networkFetch = fetch(request).then(async (response) => {
        if (response && response.ok) {
            await cache.put(url.pathname, response.clone());
        }
        return response;
    });

    if (cached) {
        networkFetch.catch(() => {});
        return cached;
    }

    try {
        return await networkFetch;
    } catch (_) {
        return cached || new Response('Sin conexion', {
            status: 503,
            statusText: 'Service Unavailable',
            headers: { 'Content-Type': 'text/plain; charset=utf-8' },
        });
    }
}

self.addEventListener('fetch', (event) => {
    const { request } = event;
    if (request.method !== 'GET') return;

    const url = new URL(request.url);
    if (url.origin !== self.location.origin) return;
    if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/auth/')) return;

    if (request.mode === 'navigate' || request.destination === 'document') {
        event.respondWith(handleNavigate(request, event.preloadResponse));
        return;
    }

    if (isStaticAssetRequest(request, url)) {
        event.respondWith(handleStaticAsset(request));
    }
});
