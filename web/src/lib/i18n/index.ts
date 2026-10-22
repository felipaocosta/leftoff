import { locales, type Locale } from './locales';
import { ptBR, type MessageKey } from './pt-BR';
import { i18n } from './state.svelte';

export type { Locale, MessageKey };
export { i18n, locales };

/**
 * Returns the text for key in the current language, replacing {name}
 * placeholders. When values.n is 1 and "<key>_one" exists, that singular
 * form is used (enough for pt-BR and en; Intl.PluralRules would treat 0 as
 * singular in Portuguese). A text missing from a translation falls back to
 * pt-BR.
 */
export function t(key: MessageKey, values: Record<string, string | number> = {}): string {
  const messages: Record<string, string> = locales[i18n.locale].messages;
  let text = messages[key] ?? ptBR[key];
  if (values.n === 1) text = messages[`${key}_one`] ?? text;
  return text.replace(/\{(\w+)\}/g, (match, name: string) =>
    name in values ? String(values[name]) : match,
  );
}
