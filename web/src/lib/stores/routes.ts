// Hash-based router: the Go server only serves static files, so routes
// live after "#" (e.g. #/series/01JAB...).

export type Route =
  | { name: 'library' }
  | { name: 'series'; id: string }
  | { name: 'add'; url?: string }
  | { name: 'news' }
  | { name: 'recipes'; domain?: string; url?: string }
  | { name: 'about' }
  | { name: 'settings' }
  | { name: 'not_found' };

/** Parses a location hash ("#/series/1") into a Route. */
export function parseHash(hash: string): Route {
  const [path = '', query = ''] = (hash.replace(/^#/, '') || '/').split('?', 2);
  const parts = path.split('/').filter(Boolean);
  if (parts.length === 1 && parts[0] === 'recipes') return recipesRoute(new URLSearchParams(query));
  if (parts.length === 1 && parts[0] === 'add') {
    const url = new URLSearchParams(query).get('url');
    return url ? { name: 'add', url } : { name: 'add' };
  }
  if (parts.length === 0) return { name: 'library' };
  if (parts[0] === 'series' && parts.length === 2 && parts[1]) {
    return { name: 'series', id: decodeURIComponent(parts[1]) };
  }
  if (parts.length === 1 && ['settings', 'news', 'about'].includes(parts[0] ?? '')) {
    return { name: parts[0] as 'settings' | 'news' | 'about' };
  }
  return { name: 'not_found' };
}

/** "#/recipes?domain=site.org&url=…" opens that site's recipe with a test URL. */
function recipesRoute(q: URLSearchParams): Route {
  const route: Route = { name: 'recipes' };
  const domain = q.get('domain');
  const url = q.get('url');
  if (domain) route.domain = domain;
  if (url) route.url = url;
  return route;
}

/** Builds the href for a route. */
export function href(route: Route): string {
  switch (route.name) {
    case 'series':
      return `#/series/${encodeURIComponent(route.id)}`;
    case 'recipes': {
      const q = new URLSearchParams();
      if (route.domain) q.set('domain', route.domain);
      if (route.url) q.set('url', route.url);
      return q.size > 0 ? `#/recipes?${q}` : '#/recipes';
    }
    case 'add':
      return route.url ? `#/add?${new URLSearchParams({ url: route.url })}` : '#/add';
    case 'news':
    case 'about':
    case 'settings':
      return `#/${route.name}`;
    default:
      return '#/';
  }
}
