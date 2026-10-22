import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { makeSeries } from '../lib/test-fixtures';
import type { Chapter, Series } from '../lib/api/types';

const api = {
  getSeries: vi.fn(),
  markSeen: vi.fn(),
  markRead: vi.fn(),
  markReadUpTo: vi.fn(),
  refreshSeries: vi.fn(),
  editSeries: vi.fn(),
  removeSeries: vi.fn(),
  changeSource: vi.fn(),
  coverUrl: (id: string, v = 0) => `c/${id}` + (v ? `?v=${v}` : ''),
};
vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));
const { default: SeriesPage } = await import('./Series.svelte');

function ch(n: number, read = false): Chapter {
  return {
    key: String(n),
    number: n,
    label: `Chapter ${n}`,
    url: `https://site.org/c/${n}`,
    alt_urls: [],
    released_at: '2026-10-0' + n,
    discovered_at: null,
    read,
    read_at: null,
  };
}

function seriesWith(overrides: Partial<Series> = {}): Series {
  return makeSeries({
    chapters: [ch(3), ch(2), ch(1, true)],
    last_read_chapter: 1,
    latest_chapter: 3,
    unread_count: 2,
    ...overrides,
  });
}

describe('Series page', () => {
  beforeEach(() => vi.resetAllMocks());

  it('shows the series, release info and a Continue link to the next unread chapter', async () => {
    api.getSeries.mockResolvedValue(seriesWith());
    render(SeriesPage, { id: '01TEST' });
    expect(await screen.findByRole('heading', { name: 'Solo Leveling' })).toBeInTheDocument();
    expect(screen.getByText(/Frequência: Semanal/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Continuar: Chapter 2' })).toHaveAttribute(
      'href',
      'https://site.org/c/2',
    );
    expect(screen.getByText('Capítulos (3)')).toBeInTheDocument();
    expect(api.markSeen).not.toHaveBeenCalled();
  });

  it('clears the new flag when opening a series with news', async () => {
    api.getSeries.mockResolvedValue(seriesWith({ has_new: true }));
    api.markSeen.mockResolvedValue(seriesWith({ has_new: false, chapters: null }));
    render(SeriesPage, { id: '01TEST' });
    await screen.findByText('Capítulos (3)');
    expect(api.markSeen).toHaveBeenCalledWith('01TEST');
  });

  it('marks a chapter optimistically and rolls back on failure', async () => {
    api.getSeries.mockResolvedValue(seriesWith());
    let reject: (e: Error) => void = () => {};
    api.markRead.mockReturnValue(new Promise((_, r) => (reject = r)));
    render(SeriesPage, { id: '01TEST' });
    const box = await screen.findByRole('checkbox', { name: 'Lido: Chapter 2' });
    await fireEvent.click(box);
    expect(box).toBeChecked(); // optimistic, before the server answers
    expect(api.markRead).toHaveBeenCalledWith('01TEST', ['2'], true);
    reject(new Error('offline'));
    await waitFor(() =>
      expect(screen.getByRole('checkbox', { name: 'Lido: Chapter 2' })).not.toBeChecked(),
    );
    expect(screen.getByRole('alert')).toHaveTextContent('offline');
  });

  it('links chapters without their own URL to the series page', async () => {
    const noLink = { ...ch(2), url: '' };
    api.getSeries.mockResolvedValue(seriesWith({ chapters: [noLink, ch(1, true)] }));
    render(SeriesPage, { id: '01TEST' });
    expect(await screen.findByRole('link', { name: 'Chapter 2' })).toHaveAttribute(
      'href',
      'https://site.org/manga/solo',
    );
    expect(screen.getByRole('link', { name: 'Continuar: Chapter 2' })).toHaveAttribute(
      'href',
      'https://site.org/manga/solo',
    );
  });

  it('marks everything up to a chapter', async () => {
    api.getSeries.mockResolvedValue(seriesWith());
    api.markReadUpTo.mockResolvedValue(
      seriesWith({ chapters: [ch(3, true), ch(2, true), ch(1, true)], unread_count: 0 }),
    );
    render(SeriesPage, { id: '01TEST' });
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Marcar todos até o capítulo 3 como lidos' }),
    );
    expect(api.markReadUpTo).toHaveBeenCalledWith('01TEST', 3);
    expect(await screen.findByText('Tudo lido')).toBeInTheDocument();
  });

  it('refreshes and reports new chapters', async () => {
    api.getSeries.mockResolvedValue(seriesWith());
    api.refreshSeries.mockResolvedValue({ new_chapters: 2, series: seriesWith() });
    render(SeriesPage, { id: '01TEST' });
    await fireEvent.click(await screen.findByRole('button', { name: 'Atualizar' }));
    expect(await screen.findByText('2 capítulos novos.')).toBeInTheDocument();
  });

  it('edits the series', async () => {
    api.getSeries.mockResolvedValue(seriesWith());
    api.editSeries.mockResolvedValue(seriesWith({ title: 'Renamed' }));
    render(SeriesPage, { id: '01TEST' });
    await fireEvent.click(await screen.findByRole('button', { name: 'Editar' }));
    await fireEvent.input(screen.getByLabelText('Título'), { target: { value: 'Renamed' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Salvar alterações' }));
    expect(api.editSeries).toHaveBeenCalledWith(
      '01TEST',
      expect.objectContaining({ title: 'Renamed' }),
    );
    expect(await screen.findByRole('heading', { name: 'Renamed' })).toBeInTheDocument();
  });

  it('removes only after confirmation', async () => {
    api.getSeries.mockResolvedValue(seriesWith());
    api.removeSeries.mockResolvedValue(undefined);
    const confirm = vi
      .spyOn(window, 'confirm')
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true);
    render(SeriesPage, { id: '01TEST' });
    const button = await screen.findByRole('button', { name: 'Remover' });
    await fireEvent.click(button);
    expect(api.removeSeries).not.toHaveBeenCalled();
    await fireEvent.click(button);
    await waitFor(() => expect(api.removeSeries).toHaveBeenCalledWith('01TEST'));
    expect(window.location.hash).toBe('#/');
    confirm.mockRestore();
  });
});

describe('Series page cover actions', () => {
  beforeEach(() => vi.resetAllMocks());

  it('refreshes the cover from the site and reloads the image', async () => {
    api.getSeries.mockResolvedValue(seriesWith());
    const refreshCover = vi.fn().mockResolvedValue(seriesWith());
    Object.assign(api, { refreshCover });
    render(SeriesPage, { id: '01TEST' });
    const img = await screen.findByRole('img', { name: 'Solo Leveling' });
    expect(img.getAttribute('src')).toBe('c/01TEST');
    await fireEvent.click(screen.getByRole('button', { name: 'Atualizar capa' }));
    expect(await screen.findByText('Capa atualizada a partir do site.')).toBeInTheDocument();
    expect(refreshCover).toHaveBeenCalledWith('01TEST');
  });

  it('uploads an image and marks the cover as custom', async () => {
    api.getSeries.mockResolvedValue(seriesWith());
    const uploaded = seriesWith({
      cover: {
        remote_url: '',
        local: 'covers/01TEST.png',
        custom: true,
        updated_at: '2026-10-20T03:00:00Z',
      },
    });
    const uploadCover = vi.fn().mockResolvedValue(uploaded);
    Object.assign(api, { uploadCover });
    render(SeriesPage, { id: '01TEST' });
    const input = (await screen.findByLabelText('Enviar imagem…')) as HTMLInputElement;
    const file = new File(['png'], 'capa.png', { type: 'image/png' });
    await fireEvent.change(input, { target: { files: [file] } });
    expect(await screen.findByText('Imagem enviada como capa.')).toBeInTheDocument();
    expect(uploadCover).toHaveBeenCalledWith('01TEST', file);
    expect(screen.getByText('Capa personalizada')).toBeInTheDocument();
  });
});

describe('Series page source change', () => {
  beforeEach(() => vi.resetAllMocks());

  async function editAddress(url: string) {
    render(SeriesPage, { id: '01TEST' });
    await fireEvent.click(await screen.findByRole('button', { name: 'Editar' }));
    await fireEvent.input(screen.getByLabelText(/Endereço da obra/), { target: { value: url } });
    await fireEvent.click(screen.getByRole('button', { name: 'Salvar alterações' }));
  }

  it('moves the series when the address changes', async () => {
    api.getSeries.mockResolvedValue(seriesWith());
    api.changeSource.mockResolvedValue(seriesWith());
    api.editSeries.mockResolvedValue(
      seriesWith({
        source: {
          url: 'https://new.org/x',
          domain: 'new.org',
          method: 'm',
          mirrors: ['https://site.org/manga/solo'],
        },
      }),
    );
    await editAddress('https://new.org/x');
    expect(api.changeSource).toHaveBeenCalledWith('01TEST', 'https://new.org/x');
    expect(await screen.findByText('Endereços anteriores: site.org')).toBeInTheDocument();
  });

  it('asks before moving to what looks like another work', async () => {
    const { ApiError } = await import('../lib/api');
    api.getSeries.mockResolvedValue(seriesWith());
    api.changeSource
      .mockRejectedValueOnce(
        new ApiError(409, 'different_series', 'change source of x to "Other": looks different'),
      )
      .mockResolvedValueOnce(seriesWith());
    api.editSeries.mockResolvedValue(seriesWith());
    const confirm = vi.spyOn(window, 'confirm').mockReturnValueOnce(true);
    await editAddress('https://other.org/y');
    await waitFor(() =>
      expect(api.changeSource).toHaveBeenLastCalledWith('01TEST', 'https://other.org/y', true),
    );
    expect(confirm).toHaveBeenCalled();
    confirm.mockRestore();
  });
});
