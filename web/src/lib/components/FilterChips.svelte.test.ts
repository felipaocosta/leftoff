import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import FilterChips from './FilterChips.svelte';
import type { SeriesQuery } from '../api/types';

function setup(initial: SeriesQuery) {
  let query = $state(initial);
  render(FilterChips, {
    get query() {
      return query;
    },
    set query(q: SeriesQuery) {
      query = q;
    },
  });
  return () => query;
}

describe('FilterChips', () => {
  it('toggles the quick filters', async () => {
    const query = setup({ q: '' });
    const fresh = screen.getByRole('button', { name: 'Com novidades' });
    expect(fresh).toHaveAttribute('aria-pressed', 'false');
    await fireEvent.click(fresh);
    expect(query().has_new).toBe(true);
    await fireEvent.click(screen.getByRole('button', { name: /Lendo/ }));
    expect(query().category).toBe('reading');
    await fireEvent.click(screen.getByRole('button', { name: /Com novidades/ }));
    expect(query().has_new).toBeUndefined();
  });

  it('lists other active filters and removes them with a tap', async () => {
    const query = setup({
      status: 'completed',
      type: 'manhwa',
      source: 'mangaread.org',
      category: 'on_hold',
    });
    for (const name of [/Completo/, /Manhwa/, /mangaread.org/, /Pausado/]) {
      expect(screen.getByRole('button', { name })).toHaveAttribute('aria-pressed', 'true');
    }
    await fireEvent.click(screen.getByRole('button', { name: /Manhwa/ }));
    expect(query().type).toBeUndefined();
    expect(screen.queryByRole('button', { name: /Manhwa/ })).toBeNull();
  });
});
