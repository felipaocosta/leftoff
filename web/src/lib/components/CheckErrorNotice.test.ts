import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { makeSeries } from '../test-fixtures';
import type { CheckError, CheckErrorCode, Health } from '../api/types';

const api = { health: vi.fn() };
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  api,
}));
const { default: CheckErrorNotice } = await import('./CheckErrorNotice.svelte');

const error = (failures: number, code: CheckErrorCode = '', message = 'boom'): CheckError => ({
  code,
  message,
  at: '2026-10-20T00:00:00Z',
  failures,
});
const health = (mode: Health['mode']) => ({
  status: 'ok',
  version: 'x',
  mode,
  llm: false,
  solvers: [],
});

describe('CheckErrorNotice', () => {
  beforeEach(() => api.health.mockResolvedValue(health('server')));

  it('links to the recipe after repeated failures of a recipe', () => {
    const s = makeSeries({ check_error: error(3) });
    s.source = {
      ...s.source,
      method: 'recipe:site.org',
      domain: 'site.org',
      url: 'https://site.org/m/1',
    };
    render(CheckErrorNotice, { series: s });
    expect(screen.getByRole('status')).toHaveTextContent('A última verificação falhou (3×): boom');
    expect(
      screen.getByRole('link', { name: 'Revisar ou gerar a receita de novo' }),
    ).toHaveAttribute('href', '#/recipes?domain=site.org&url=https%3A%2F%2Fsite.org%2Fm%2F1');
  });

  it('shows no recipe link for other methods or few failures, and nothing without an error', () => {
    const adapter = makeSeries({ check_error: error(5) });
    const { unmount } = render(CheckErrorNotice, { series: adapter });
    expect(screen.queryByRole('link')).toBeNull();
    unmount();
    const s = makeSeries({ check_error: error(2) });
    s.source = { ...s.source, method: 'recipe:site.org' };
    const second = render(CheckErrorNotice, { series: s });
    expect(screen.queryByRole('link')).toBeNull();
    second.unmount();
    render(CheckErrorNotice, { series: makeSeries() });
    expect(screen.queryByRole('status')).toBeNull();
  });

  it('explains a page that needs a browser for the device in use', async () => {
    const technical = 'x: the page needs a browser to show its content';
    api.health.mockResolvedValue(health('android'));
    const { unmount } = render(CheckErrorNotice, {
      series: makeSeries({ check_error: error(4, 'needs_browser', technical) }),
    });
    expect(await screen.findByText(/o celular não tem um/)).toBeInTheDocument();
    expect(screen.getByText(technical)).not.toBeVisible(); // under "Detalhes"
    unmount();
    api.health.mockResolvedValue(health('desktop'));
    render(CheckErrorNotice, { series: makeSeries({ check_error: error(1, 'needs_browser') }) });
    expect(await screen.findByText(/Instale o Chrome/)).toBeInTheDocument();
  });

  it('explains the other kinds of failure', () => {
    render(CheckErrorNotice, {
      series: makeSeries({ check_error: error(4, 'source_changed', '0 of 50') }),
    });
    expect(screen.getByRole('status')).toHaveTextContent(/nada foi apagado/);
  });
});
