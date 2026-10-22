// LeftOff service worker (PWA, RNF-06: read the library offline).
// Hashed UI assets: cache first. API reads and pages: network first, falling
// back to the last copy when offline, marked with X-MT-Offline so the app
// tells the user the data may be old. Writes are never cached.
const CACHE = 'mt-v1';

self.addEventListener('install', () => self.skipWaiting());

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  const url = new URL(req.url);
  if (req.method !== 'GET' || url.origin !== self.location.origin) return;
  if (url.pathname.includes('/api/v1/export') || url.pathname.includes('/api/v1/jobs/')) return;

  if (url.pathname.includes('/assets/') || url.pathname.includes('/api/v1/covers/')) {
    event.respondWith(cacheFirst(req));
  } else {
    event.respondWith(networkFirst(req));
  }
});

async function cacheFirst(req) {
  const hit = await caches.match(req);
  if (hit) return hit;
  const res = await fetch(req);
  if (res.ok) (await caches.open(CACHE)).put(req, res.clone());
  return res;
}

async function networkFirst(req) {
  try {
    const res = await fetch(req);
    if (res.ok) (await caches.open(CACHE)).put(req, res.clone());
    return res;
  } catch (err) {
    const hit =
      (await caches.match(req)) ?? (req.mode === 'navigate' ? await caches.match('./') : undefined);
    if (hit) return markOffline(hit);
    throw err;
  }
}

// markOffline copies a cached response with the X-MT-Offline header.
function markOffline(res) {
  const headers = new Headers(res.headers);
  headers.set('X-MT-Offline', '1');
  return new Response(res.body, { status: res.status, statusText: res.statusText, headers });
}
