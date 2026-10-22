import { beforeEach, describe, expect, it, vi } from 'vitest';
import { applyTheme, prefs } from './prefs.svelte';

describe('prefs', () => {
  beforeEach(() => localStorage.clear());

  it('persists a trimmed token and clears it', () => {
    prefs.setToken('  abc ');
    expect(prefs.token).toBe('abc');
    expect(localStorage.getItem('mt.token')).toBe('abc');
    prefs.setToken('');
    expect(localStorage.getItem('mt.token')).toBeNull();
  });

  it('applies and persists the theme', () => {
    prefs.setTheme('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(localStorage.getItem('mt.theme')).toBe('dark');
    prefs.setTheme('system');
    expect(document.documentElement.dataset.theme).toBeUndefined();
    expect(localStorage.getItem('mt.theme')).toBeNull();
    applyTheme('light');
    expect(document.documentElement.dataset.theme).toBe('light');
  });

  it('persists the library view, grid being the default', () => {
    prefs.setView('list');
    expect(prefs.view).toBe('list');
    expect(localStorage.getItem('mt.view')).toBe('list');
    prefs.setView('grid');
    expect(localStorage.getItem('mt.view')).toBeNull();
  });

  it('tells the Android app whether the theme in use is dark', () => {
    const setDarkTheme = vi.fn();
    (window as unknown as { LeftOffAndroid: unknown }).LeftOffAndroid = { setDarkTheme };
    try {
      prefs.setTheme('dark');
      prefs.setTheme('light');
      expect(setDarkTheme.mock.calls).toEqual([[true], [false]]);
    } finally {
      delete (window as unknown as { LeftOffAndroid?: unknown }).LeftOffAndroid;
    }
  });
});
