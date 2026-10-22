// User preferences kept in this browser (localStorage): the API token, the
// theme and the library view (grid or list, RF-31). Storage may be unavailable (private mode), so every access is
// guarded and the app works with the defaults.

export type Theme = 'system' | 'light' | 'dark';
export type LibraryView = 'grid' | 'list';

const TOKEN_KEY = 'mt.token';
const THEME_KEY = 'mt.theme';
const VIEW_KEY = 'mt.view';
const FILTERS_KEY = 'mt.filters';

/** Filters start open on wide screens and closed on phones. */
function filtersOpenDefault(): boolean {
  const saved = read(FILTERS_KEY);
  if (saved) return saved === 'open';
  return typeof window === 'undefined' || !window.matchMedia?.('(max-width: 640px)').matches;
}

function read(key: string): string {
  try {
    return localStorage.getItem(key) ?? '';
  } catch {
    return '';
  }
}

function write(key: string, value: string): void {
  try {
    if (value) localStorage.setItem(key, value);
    else localStorage.removeItem(key);
  } catch {
    // Storage unavailable: the value lives only for this session.
  }
}

function isTheme(v: string): v is Theme {
  return v === 'system' || v === 'light' || v === 'dark';
}

class Prefs {
  token = $state(read(TOKEN_KEY));
  theme = $state<Theme>(isTheme(read(THEME_KEY)) ? (read(THEME_KEY) as Theme) : 'system');
  view = $state<LibraryView>(read(VIEW_KEY) === 'list' ? 'list' : 'grid');
  filtersOpen = $state(filtersOpenDefault());

  setFiltersOpen(open: boolean): void {
    this.filtersOpen = open;
    write(FILTERS_KEY, open ? 'open' : 'closed');
  }

  setView(view: LibraryView): void {
    this.view = view;
    write(VIEW_KEY, view === 'grid' ? '' : view);
  }

  setToken(token: string): void {
    this.token = token.trim();
    write(TOKEN_KEY, this.token);
  }

  setTheme(theme: Theme): void {
    this.theme = theme;
    write(THEME_KEY, theme === 'system' ? '' : theme);
    applyTheme(theme);
  }
}

/** Sets data-theme on <html>; "system" follows prefers-color-scheme. */
export function applyTheme(theme: Theme): void {
  if (theme === 'system') delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = theme;
  appliedTheme = theme;
  syncSystemBars();
}

// The Android app paints its status and navigation bars to match the
// theme in use (its bridge is absent elsewhere).
type AndroidBridge = { setDarkTheme?: (dark: boolean) => void };
let appliedTheme: Theme = 'system';
const darkQuery =
  typeof window !== 'undefined' ? window.matchMedia?.('(prefers-color-scheme: dark)') : undefined;
darkQuery?.addEventListener?.('change', () => syncSystemBars());

function syncSystemBars(): void {
  const bridge = (window as { LeftOffAndroid?: AndroidBridge }).LeftOffAndroid;
  if (!bridge?.setDarkTheme) return;
  bridge.setDarkTheme(
    appliedTheme === 'dark' || (appliedTheme === 'system' && !!darkQuery?.matches),
  );
}

export const prefs = new Prefs();
