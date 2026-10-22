import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../api/client';
import type { SettingsView } from '../api/types';

const api = { getSettings: vi.fn(), saveSettings: vi.fn(), testNotifications: vi.fn() };
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  api,
}));
const { default: NotificationSettings } = await import('./NotificationSettings.svelte');

const stored: SettingsView = {
  notifications: {
    ntfy_url: 'https://ntfy.sh/manga-x',
    discord_webhook: '••••abcd',
    telegram: { token: '', chat_id: '42' },
    only_reading: false,
    language: 'pt-BR',
    locked: ['telegram.chat_id'],
  },
  enrichment: { enabled: false },
};

describe('NotificationSettings', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    api.getSettings.mockResolvedValue(stored);
    api.saveSettings.mockImplementation(async (v: SettingsView) => v);
  });

  it('shows the stored values, locks env fields and saves with masked secrets', async () => {
    render(NotificationSettings);
    const ntfy = (await screen.findByLabelText(/ntfy: endereço do tópico/)) as HTMLInputElement;
    expect(ntfy.value).toBe('https://ntfy.sh/manga-x');
    expect(screen.getByLabelText(/Telegram: id da conversa/)).toBeDisabled();
    expect(screen.getByText('Definido por variável de ambiente no servidor')).toBeInTheDocument();

    await fireEvent.click(screen.getByLabelText('Só obras na categoria "Lendo"'));
    await fireEvent.click(screen.getByRole('button', { name: 'Salvar notificações' }));
    expect(await screen.findByText('Notificações salvas.')).toBeInTheDocument();
    expect(api.saveSettings).toHaveBeenCalledWith({
      notifications: {
        ntfy_url: 'https://ntfy.sh/manga-x',
        discord_webhook: '••••abcd',
        telegram: { token: '', chat_id: '42' },
        only_reading: true,
        language: 'pt-BR',
      },
    });
  });

  it('saves before testing and shows each channel result', async () => {
    api.testNotifications.mockResolvedValue([
      { channel: 'ntfy', ok: true },
      { channel: 'discord', ok: false, error: 'discord: HTTP 401' },
    ]);
    render(NotificationSettings);
    await fireEvent.click(await screen.findByRole('button', { name: 'Enviar teste' }));
    expect(await screen.findByText('ntfy: mensagem enviada.')).toBeInTheDocument();
    expect(screen.getByText('discord: falhou (discord: HTTP 401).')).toBeInTheDocument();
    expect(api.saveSettings).toHaveBeenCalledBefore(api.testNotifications);
  });

  it('shows validation errors from the server', async () => {
    api.saveSettings.mockRejectedValue(
      new ApiError(400, 'invalid_input', 'ntfy_url must be http(s)://server/topic'),
    );
    render(NotificationSettings);
    await fireEvent.input(await screen.findByLabelText(/ntfy: endereço do tópico/), {
      target: { value: 'nope' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Salvar notificações' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('ntfy_url must be');
  });
});
