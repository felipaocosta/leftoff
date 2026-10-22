import { afterEach, describe, expect, it } from 'vitest';
import { formatDate } from '../format';
import { i18n, locales, t, type Locale } from './index';
import { detectLocale } from './locales';
import { ptBR } from './pt-BR';

const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

describe('t', () => {
  afterEach(() => i18n.set('pt-BR'));

  it('returns the text of the current language', () => {
    expect(t('nav.settings')).toBe('Configurações');
    i18n.set('en');
    expect(t('nav.settings')).toBe('Settings');
    expect(document.documentElement.lang).toBe('en');
    expect(localStorage.getItem('mt.lang')).toBe('en');
  });

  it('fills placeholders and uses the singular form for n = 1', () => {
    expect(t('recipes.chapter_count', { n: 2 })).toBe('2 capítulos');
    expect(t('recipes.chapter_count', { n: 1 })).toBe('1 capítulo');
    expect(t('library.count', { n: 0 })).toBe('0 obras');
    i18n.set('en');
    expect(t('release.overdue', { n: 1, date: 'x' })).toBe('1 day late (due x)');
    expect(t('chapter.short', {})).toBe('Ch. {n}'); // unknown values stay visible
  });

  it('formats dates in the current language', () => {
    expect(formatDate('2026-10-22')).toBe('22 de out. de 2026');
    i18n.set('en');
    expect(formatDate('2026-10-22')).toBe('Oct 22, 2026');
  });
});

describe('translations', () => {
  const keys = Object.keys(ptBR) as (keyof typeof ptBR)[];

  for (const locale of Object.keys(locales) as Locale[]) {
    it(`${locale} has the same keys and placeholders as pt-BR`, () => {
      const messages: Record<string, string> = locales[locale].messages;
      expect(Object.keys(messages).sort()).toEqual([...keys].sort());
      for (const key of keys) {
        expect(placeholders(messages[key] ?? ''), `${locale} ${key}`).toEqual(
          placeholders(ptBR[key]),
        );
      }
    });
  }

  it('only has singular forms for keys that exist and take {n}', () => {
    for (const key of keys.filter((k) => k.endsWith('_one'))) {
      const base = key.slice(0, -'_one'.length) as keyof typeof ptBR;
      expect(ptBR[base], key).toContain('{n}');
    }
  });
});

describe('detectLocale', () => {
  it.each([
    ['en', ['pt-BR'], 'en'],
    ['', ['en-US', 'pt-BR'], 'en'],
    ['', ['pt-PT'], 'pt-BR'],
    ['', ['fr-FR', 'de'], 'pt-BR'],
    ['xx', [], 'pt-BR'],
  ])('stored %j, browser %j → %s', (stored, browser, want) => {
    expect(detectLocale(stored, browser)).toBe(want);
  });
});
