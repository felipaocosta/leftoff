// The current route, following the location hash. Parsing and building
// hashes are pure functions in routes.ts.
import { parseHash, href, type Route } from './routes';

export { parseHash, href, type Route };

class Router {
  current = $state<Route>(parseHash(window.location.hash));

  constructor() {
    window.addEventListener('hashchange', () => {
      this.current = parseHash(window.location.hash);
      window.scrollTo(0, 0);
    });
  }

  go(route: Route): void {
    window.location.hash = href(route);
  }
}

export const router = new Router();
