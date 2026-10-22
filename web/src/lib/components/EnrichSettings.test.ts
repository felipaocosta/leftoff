import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const api = { getSettings: vi.fn(), saveSettings: vi.fn() };
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  api,
}));
const { default: EnrichSettings } = await import('./EnrichSettings.svelte');

describe('EnrichSettings', () => {
  beforeEach(() => vi.resetAllMocks());

  it('saves only the enrichment section', async () => {
    api.getSettings.mockResolvedValue({ enrichment: { enabled: false } });
    api.saveSettings.mockResolvedValue({ enrichment: { enabled: true } });
    render(EnrichSettings);
    const box = await screen.findByLabelText(/Buscar automaticamente/);
    expect(box).not.toBeChecked();
    await fireEvent.click(box);
    expect(api.saveSettings).toHaveBeenCalledWith({ enrichment: { enabled: true } });
    expect(await screen.findByText('Preferência salva.')).toBeInTheDocument();
  });

  it('is locked by ENRICH_ENABLED', async () => {
    api.getSettings.mockResolvedValue({ enrichment: { enabled: true, locked: true } });
    render(EnrichSettings);
    expect(await screen.findByLabelText(/Buscar automaticamente/)).toBeDisabled();
    expect(screen.getByText('Definido por ENRICH_ENABLED no servidor')).toBeInTheDocument();
  });
});
