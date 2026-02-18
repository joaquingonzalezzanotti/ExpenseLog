self.addEventListener('install', () => {
    self.skipWaiting();
});

self.addEventListener('activate', (event) => {
    event.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', (event) => {
    event.respondWith((async () => {
        try {
            return await fetch(event.request);
        } catch (error) {
            const cached = await caches.match(event.request);
            if (cached) return cached;

            if (event.request.mode === 'navigate') {
                const fallback = await caches.match('/app') || await caches.match('/');
                if (fallback) return fallback;
            }

            return new Response('Sin conexion', {
                status: 503,
                statusText: 'Service Unavailable',
                headers: { 'Content-Type': 'text/plain; charset=utf-8' },
            });
        }
    })());
});
