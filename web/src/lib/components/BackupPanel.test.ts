import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const api = { exportLibrary: vi.fn(), importLibrary: vi.fn(), saveExportDesktop: vi.fn() };
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  api,
}));
const { default: BackupPanel } = await import('./BackupPanel.svelte');
const { ApiError } = await import('../api');
const { toasts } = await import('../stores/toast.svelte');

describe('BackupPanel', () => {
  beforeEach(() => vi.resetAllMocks());

  it('downloads the export in a browser and says so', async () => {
    api.exportLibrary.mockResolvedValue({
      blob: new Blob(['{}']),
      filename: 'leftoff-x.json',
    });
    URL.createObjectURL = vi.fn(() => 'blob:x');
    URL.revokeObjectURL = vi.fn();
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    render(BackupPanel);
    await fireEvent.click(screen.getByRole('button', { name: 'Exportar biblioteca' }));
    await vi.waitFor(() => expect(click).toHaveBeenCalled());
    expect((click.mock.contexts[0] as HTMLAnchorElement).download).toBe('leftoff-x.json');
    expect(toasts.items.at(-1)?.text).toMatch(/Exportação baixada: leftoff-x.json/);
    click.mockRestore();
  });

  it('lets the Android app pick the folder and reports the result', async () => {
    api.exportLibrary.mockResolvedValue({ blob: new Blob(['{"series":[]}']), filename: 'mt.json' });
    const saveFile = vi.fn((name: string) => {
      window.dispatchEvent(
        new CustomEvent('mt-file-saved', { detail: { ok: true, name: 'Docs/' + name } }),
      );
    });
    (window as unknown as { LeftOffAndroid: unknown }).LeftOffAndroid = { saveFile };
    try {
      render(BackupPanel);
      await fireEvent.click(screen.getByRole('button', { name: 'Exportar biblioteca' }));
      await vi.waitFor(() =>
        expect(toasts.items.at(-1)?.text).toBe('Biblioteca exportada: Docs/mt.json'),
      );
      expect(saveFile).toHaveBeenCalledWith('mt.json', '{"series":[]}');

      saveFile.mockImplementation(() => {
        window.dispatchEvent(
          new CustomEvent('mt-file-saved', { detail: { ok: false, canceled: true } }),
        );
      });
      await fireEvent.click(screen.getByRole('button', { name: 'Exportar biblioteca' }));
      await vi.waitFor(() => expect(toasts.items.at(-1)?.text).toBe('Exportação cancelada.'));
    } finally {
      delete (window as unknown as { LeftOffAndroid?: unknown }).LeftOffAndroid;
    }
  });

  it('uses the native dialog in the desktop app', async () => {
    api.saveExportDesktop.mockResolvedValue({ canceled: false, path: '/home/u/mt.json' });
    render(BackupPanel, { mode: 'desktop' });
    await fireEvent.click(screen.getByRole('button', { name: 'Exportar biblioteca' }));
    await vi.waitFor(() =>
      expect(toasts.items.at(-1)?.text).toBe('Biblioteca exportada: /home/u/mt.json'),
    );
    expect(api.exportLibrary).not.toHaveBeenCalled();
  });

  it('imports a file and shows the summary or the error', async () => {
    api.importLibrary.mockResolvedValueOnce({ added: 2, merged: 1, chapters_read: 5 });
    render(BackupPanel);
    const input = screen.getByLabelText('Importar arquivo…') as HTMLInputElement;
    const file = new File(['{"schema":1,"series":[]}'], 'backup.json', {
      type: 'application/json',
    });
    await fireEvent.change(input, { target: { files: [file] } });
    expect(await screen.findByRole('status')).toHaveTextContent(
      '2 obras novas, 1 juntadas, 5 capítulos',
    );
    expect(api.importLibrary).toHaveBeenCalledWith('{"schema":1,"series":[]}');
    expect(toasts.items.at(-1)?.text).toMatch(/Importação concluída: 2 obras novas/); // floating message too

    api.importLibrary.mockRejectedValueOnce(
      new ApiError(400, 'invalid_input', 'not a LeftOff export'),
    );
    await fireEvent.change(input, { target: { files: [file] } });
    expect(await screen.findByRole('alert')).toHaveTextContent('not a LeftOff export');
    expect(toasts.items.at(-1)?.kind).toBe('error');
  });
});
