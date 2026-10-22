import { i18n, t, type Locale } from './i18n';
import type { DateString } from './api/types';

const DAY_MS = 24 * 60 * 60 * 1000;

// Formatters follow the UI language; built once per language.
const numberFormats = new Map<Locale, Intl.NumberFormat>();
const dateFormats = new Map<Locale, Intl.DateTimeFormat>();

function numberFormat(): Intl.NumberFormat {
  let f = numberFormats.get(i18n.locale);
  if (!f) {
    f = new Intl.NumberFormat(i18n.locale, { maximumFractionDigits: 2 });
    numberFormats.set(i18n.locale, f);
  }
  return f;
}

function dateFormat(): Intl.DateTimeFormat {
  let f = dateFormats.get(i18n.locale);
  if (!f) {
    f = new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium', timeZone: 'UTC' });
    dateFormats.set(i18n.locale, f);
  }
  return f;
}

/** A date and time in the UI language and the browser's time zone. */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '';
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString(i18n.locale);
}

/** "123", "10,5" or "—" for a missing chapter number. */
export function formatChapter(n: number | null | undefined): string {
  return n == null ? t('chapter.none') : numberFormat().format(n);
}

/** Formats a "YYYY-MM-DD" date for display ("22 de out. de 2026"). */
export function formatDate(d: DateString | null | undefined): string {
  if (!d) return '';
  const date = new Date(d + 'T00:00:00Z');
  return Number.isNaN(date.getTime()) ? '' : dateFormat().format(date);
}

/** Human label for a release interval in days (RF-27). */
export function frequencyLabel(days: number | null | undefined): string {
  if (days == null) return t('freq.unknown');
  if (days <= 1.5) return t('freq.daily');
  if (days >= 6 && days <= 8) return t('freq.weekly');
  if (days >= 13 && days <= 16) return t('freq.biweekly');
  if (days >= 28 && days <= 32) return t('freq.monthly');
  return t('freq.every', { n: numberFormat().format(Math.round(days)) });
}

export type ReleaseState =
  | { kind: 'none' }
  | { kind: 'upcoming'; days: number }
  | { kind: 'today' }
  | { kind: 'overdue'; days: number };

/** Compares the next release estimate with today (both as UTC dates). */
export function releaseState(next: DateString | null | undefined, today: Date): ReleaseState {
  if (!next) return { kind: 'none' };
  const target = Date.parse(next + 'T00:00:00Z');
  if (Number.isNaN(target)) return { kind: 'none' };
  const start = Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate());
  const days = Math.round((target - start) / DAY_MS);
  if (days === 0) return { kind: 'today' };
  return days > 0 ? { kind: 'upcoming', days } : { kind: 'overdue', days: -days };
}

/** Text for the next release estimate, e.g. "Atrasado há 3 dias (…)". */
export function releaseLabel(next: DateString | null | undefined, today: Date): string {
  const state = releaseState(next, today);
  const date = formatDate(next);
  switch (state.kind) {
    case 'none':
      return '';
    case 'today':
      return t('release.today');
    case 'upcoming':
      return state.days === 1
        ? t('release.tomorrow', { date })
        : t('release.in', { n: state.days, date });
    case 'overdue':
      return t('release.overdue', { n: state.days, date });
  }
}
