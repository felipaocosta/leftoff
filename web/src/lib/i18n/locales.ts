import { en } from './en';
import { ptBR, type Messages } from './pt-BR';

/**
 * The UI languages. To add one, write <tag>.ts typed as Messages (the
 * compiler lists any missing key) and register it here.
 */
export const locales = {
  'pt-BR': { name: 'Português (Brasil)', messages: ptBR as Messages },
  en: { name: 'English', messages: en },
} as const;

export type Locale = keyof typeof locales;

export const defaultLocale: Locale = 'pt-BR';

export function isLocale(v: string): v is Locale {
  return Object.hasOwn(locales, v);
}

/**
 * Picks the stored choice, else the first browser language we have (exact
 * tag, then the base language: "en-US" → "en", "pt-PT" → "pt-BR"), else
 * the default.
 */
export function detectLocale(stored: string, browser: readonly string[]): Locale {
  if (isLocale(stored)) return stored;
  for (const tag of browser) {
    if (isLocale(tag)) return tag;
    const base = tag.split('-')[0]?.toLowerCase() ?? '';
    const match = (Object.keys(locales) as Locale[]).find((l) => l.split('-')[0] === base);
    if (match) return match;
  }
  return defaultLocale;
}
