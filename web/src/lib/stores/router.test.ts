import { describe, expect, it } from 'vitest';
import { href, parseHash } from './router.svelte';

describe('parseHash', () => {
  it.each([
    ['', { name: 'library' }],
    ['#/', { name: 'library' }],
    ['#/series/01JAB', { name: 'series', id: '01JAB' }],
    ['#/series/a%20b', { name: 'series', id: 'a b' }],
    ['#/add', { name: 'add' }],
    ['#/settings', { name: 'settings' }],
    ['#/news', { name: 'news' }],
    ['#/recipes', { name: 'recipes' }],
    ['#/recipes?domain=a.org', { name: 'recipes', domain: 'a.org' }],
    ['#/about', { name: 'about' }],
    ['#/add?url=https%3A%2F%2Fa.org%2Fm%2F1', { name: 'add', url: 'https://a.org/m/1' }],
    ['#/series', { name: 'not_found' }],
    ['#/nope', { name: 'not_found' }],
  ])('%s', (hash, want) => {
    expect(parseHash(hash)).toEqual(want);
  });

  it('round-trips with href', () => {
    expect(parseHash(href({ name: 'series', id: 'x/y' }))).toEqual({ name: 'series', id: 'x/y' });
    expect(href({ name: 'library' })).toBe('#/');
    const r = { name: 'recipes', domain: 'a.org', url: 'https://a.org/m/x?y=1&z' } as const;
    expect(parseHash(href(r))).toEqual(r);
    expect(href({ name: 'recipes' })).toBe('#/recipes');
  });
});
