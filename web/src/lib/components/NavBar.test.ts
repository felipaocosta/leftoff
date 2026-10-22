import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

vi.mock('../stores/news.svelte', () => ({ news: { count: 3, refresh: vi.fn() } }));
const { default: NavBar } = await import('./NavBar.svelte');

describe('NavBar', () => {
  it('lists every section, marks the current one and shows the news count', () => {
    render(NavBar, { current: 'settings' });
    const nav = screen.getByRole('navigation', { name: 'Navegação principal' });
    for (const name of ['Biblioteca', 'Novidades', 'Adicionar', 'Configurações', 'Sobre']) {
      expect(nav).toHaveTextContent(name);
    }
    expect(screen.getByRole('link', { name: /Configurações/ })).toHaveAttribute(
      'aria-current',
      'page',
    );
    expect(screen.getByLabelText('Novidades (3)')).toHaveTextContent('3');
  });
});
