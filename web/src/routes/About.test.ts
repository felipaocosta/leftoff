import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const api = { health: vi.fn() };
vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));
const { default: About } = await import('./About.svelte');

describe('About', () => {
  beforeEach(() => vi.resetAllMocks());

  it('explains how the app works and shows the server status', async () => {
    api.health.mockResolvedValue({
      status: 'ok',
      version: 'v9',
      mode: 'server',
      llm: true,
      solvers: ['flaresolverr', 'browser'],
    });
    render(About);
    expect(
      screen.getByRole('heading', { level: 1, name: 'Sobre o LeftOff (Onde Parei)' }),
    ).toBeInTheDocument();
    for (const name of ['Como ele lê um site', 'Receitas e IA', 'Seus dados', 'Idiomas']) {
      expect(screen.getByRole('heading', { name })).toBeInTheDocument();
    }
    expect(screen.getByText(/A IA nunca altera a sua biblioteca/)).toBeInTheDocument();
    const status = await screen.findByText(/Versão v9 · servidor/);
    expect(status).toHaveTextContent('IA: ligada');
    expect(status).toHaveTextContent('Cloudflare): FlareSolverr, navegador deste computador.');
  });

  it('still shows the texts when the server is unreachable', async () => {
    api.health.mockRejectedValue(new TypeError('offline'));
    render(About);
    expect(screen.getByRole('heading', { name: 'Atualizações' })).toBeInTheDocument();
    await Promise.resolve();
    expect(screen.queryByText(/Versão/)).not.toBeInTheDocument();
  });

  it('names the phone app and credits the author', async () => {
    api.health.mockResolvedValue({
      status: 'ok',
      version: 'v9',
      mode: 'android',
      llm: false,
      solvers: ['webview'],
    });
    render(About);
    const status = await screen.findByText(/Versão v9 · aplicativo neste celular/);
    expect(status).toHaveTextContent('navegador interno do celular');
    expect(
      screen.getByText('Feito por FCR, com o auxílio do Claude Opus 5.5.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'fcostarodrigues@gmail.com' })).toHaveAttribute(
      'href',
      'mailto:fcostarodrigues@gmail.com',
    );
    expect(screen.getByText(/GPL-3.0 ou posterior/)).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: 'https://github.com/felipaocosta/leftoff' }),
    ).toBeInTheDocument();
  });
});
