import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const api = { health: vi.fn(), quitApp: vi.fn(), getSettings: vi.fn() };
vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));
const { default: Settings } = await import('./Settings.svelte');

describe('Settings', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    api.getSettings.mockResolvedValue({
      notifications: {
        ntfy_url: '',
        discord_webhook: '',
        telegram: { token: '', chat_id: '' },
        only_reading: false,
        language: 'pt-BR',
      },
      enrichment: { enabled: false },
    });
  });

  it('offers quitting only in app mode', async () => {
    api.health.mockResolvedValue({ status: 'ok', version: 'v1', mode: 'app' });
    api.quitApp.mockResolvedValue(undefined);
    render(Settings);
    await fireEvent.click(await screen.findByRole('button', { name: 'Sair do app' }));
    expect(api.quitApp).toHaveBeenCalled();
    expect(await screen.findByText(/App encerrado/)).toBeInTheDocument();
  });

  it('hides quitting in server mode and saves the token', async () => {
    api.health.mockResolvedValue({ status: 'ok', version: 'v1', mode: 'server' });
    render(Settings);
    expect(await screen.findByText('Servidor: versão v1')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sair do app' })).not.toBeInTheDocument();
    await fireEvent.input(screen.getByLabelText(/Token de acesso/), { target: { value: 'abc' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Salvar' }));
    expect(screen.getByText('Salvo.')).toBeInTheDocument();
    expect(localStorage.getItem('mt.token')).toBe('abc');
  });

  it('switches the whole UI to English', async () => {
    api.health.mockResolvedValue({ status: 'ok', version: 'v1', mode: 'server', llm: false });
    render(Settings);
    await fireEvent.click(screen.getByLabelText('English'));
    expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument();
    expect(screen.getByText('Server: version v1')).toBeInTheDocument();
    await fireEvent.click(screen.getByLabelText('Português (Brasil)'));
    expect(await screen.findByRole('heading', { name: 'Configurações' })).toBeInTheDocument();
  });
});
