import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const api = { getSettings: vi.fn(), saveSettings: vi.fn() };
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  api,
}));
const { default: DesktopMode } = await import('./DesktopMode.svelte');

describe('DesktopMode', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    api.getSettings.mockResolvedValue({ desktop: { mode: 'standalone', server_url: '' } });
  });

  it('connects to a server and opens it in the window', async () => {
    api.saveSettings.mockResolvedValue({
      desktop: { mode: 'connected', server_url: 'http://nas:8080' },
    });
    const navigate = vi.fn();
    render(DesktopMode, { navigate });
    await fireEvent.click(await screen.findByLabelText('Conectar a um servidor'));
    await fireEvent.input(screen.getByLabelText('Endereço do servidor'), {
      target: { value: 'http://nas:8080' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Aplicar' }));
    expect(await screen.findByText('Modo salvo.')).toBeInTheDocument();
    expect(api.saveSettings).toHaveBeenCalledWith({
      desktop: { mode: 'connected', server_url: 'http://nas:8080' },
    });
    expect(navigate).toHaveBeenCalledWith('http://nas:8080/');
  });

  it('stays here in standalone mode', async () => {
    api.saveSettings.mockResolvedValue({ desktop: { mode: 'standalone', server_url: '' } });
    const navigate = vi.fn();
    render(DesktopMode, { navigate });
    await fireEvent.click(await screen.findByRole('button', { name: 'Aplicar' }));
    expect(await screen.findByText('Modo salvo.')).toBeInTheDocument();
    expect(navigate).not.toHaveBeenCalled();
  });
});
