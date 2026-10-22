// The current UI language, kept in this browser (localStorage, like the
// theme). Reading i18n.locale inside a template makes it re-render when
// the language changes.
import { defaultLocale, detectLocale, type Locale } from './locales';

const KEY = 'mt.lang';

function stored(): string {
  try {
    return localStorage.getItem(KEY) ?? '';
  } catch {
    return '';
  }
}

function browserLanguages(): readonly string[] {
  return typeof navigator === 'undefined' ? [] : navigator.languages;
}

class I18n {
  locale = $state<Locale>(detectLocale(stored(), browserLanguages()));

  constructor() {
    applyLang(this.locale);
  }

  set(locale: Locale): void {
    this.locale = locale;
    try {
      localStorage.setItem(KEY, locale);
    } catch {
      // Storage unavailable: the choice lasts for this session.
    }
    applyLang(locale);
  }
}

/** Keeps <html lang> right for screen readers and hyphenation. */
function applyLang(locale: Locale): void {
  if (typeof document !== 'undefined') document.documentElement.lang = locale;
}

export const i18n = new I18n();
export { defaultLocale };
