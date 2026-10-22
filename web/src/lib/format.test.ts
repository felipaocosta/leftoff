import { describe, expect, it } from 'vitest';
import { formatChapter, formatDate, frequencyLabel, releaseLabel, releaseState } from './format';

const today = new Date('2026-10-19T15:00:00Z');

describe('formatChapter', () => {
  it.each([
    [245, '245'],
    [10.5, '10,5'],
    [0, '0'],
    [null, '—'],
  ])('%s → %s', (n, want) => expect(formatChapter(n)).toBe(want));
});

describe('frequencyLabel', () => {
  it.each([
    [1, 'Diário'],
    [7, 'Semanal'],
    [6.5, 'Semanal'],
    [14, 'Quinzenal'],
    [30, 'Mensal'],
    [4, 'A cada 4 dias'],
    [3.5, 'A cada 4 dias'],
    [null, 'Sem dados suficientes'],
  ])('%s → %s', (days, want) => expect(frequencyLabel(days)).toBe(want));
});

describe('releaseState', () => {
  it.each([
    ['2026-10-22', { kind: 'upcoming', days: 3 }],
    ['2026-10-19', { kind: 'today' }],
    ['2026-04-17', { kind: 'overdue', days: 185 }],
    [null, { kind: 'none' }],
    ['garbage', { kind: 'none' }],
  ])('%s', (next, want) => expect(releaseState(next, today)).toEqual(want));
});

describe('releaseLabel', () => {
  it('describes upcoming, tomorrow, today and overdue releases', () => {
    expect(releaseLabel('2026-10-22', today)).toMatch(/^Previsto em 3 dias \(22 de out/);
    expect(releaseLabel('2026-10-20', today)).toMatch(/^Previsto para amanhã/);
    expect(releaseLabel('2026-10-19', today)).toBe('Previsto para hoje');
    expect(releaseLabel('2026-10-12', today)).toMatch(/^Atrasado há 7 dias/);
    expect(releaseLabel(null, today)).toBe('');
  });
});

describe('formatDate', () => {
  it('formats in UTC and ignores invalid values', () => {
    expect(formatDate('2026-10-22')).toMatch(/22 de out\.? de 2026/);
    expect(formatDate('')).toBe('');
    expect(formatDate('nope')).toBe('');
  });
});
