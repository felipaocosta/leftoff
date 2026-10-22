import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { makeSeries } from '../test-fixtures';
import type { External } from '../api/types';

const api = { enrichSeries: vi.fn(), unlinkSeries: vi.fn(), editSeries: vi.fn() };
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  api,
}));
const { default: ExternalInfo } = await import('./ExternalInfo.svelte');
const { default: SeriesAbout } = await import('./SeriesAbout.svelte');

const anilist: External = {
  source: 'anilist',
  id: '106929',
  url: 'https://anilist.co/manga/106929',
  title: 'Eleceed',
  status: 'ongoing',
  genres: ['Action', 'Comedy'],
  year: 2018,
  synopsis: 'Kaiden, um gato…',
  ignored: false,
  checked_at: '2026-10-20T12:00:00Z',
};

describe('ExternalInfo', () => {
  beforeEach(() => vi.resetAllMocks());

  it('shows the official status, adds genres to tags and unlinks', async () => {
    const series = makeSeries({ tags: ['action'], external: anilist });
    const onChange = vi.fn();
    api.editSeries.mockResolvedValue({ ...series, tags: ['action', 'Comedy'] });
    api.unlinkSeries.mockResolvedValue({
      ...series,
      external: { ...anilist, id: '', ignored: true },
    });
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    render(ExternalInfo, { series, onChange });
    expect(screen.getByText(/Status oficial \(AniList\): Em andamento/)).toHaveTextContent(
      'desde 2018',
    );
    expect(screen.getByRole('link', { name: 'Abrir no AniList' })).toHaveAttribute(
      'href',
      anilist.url,
    );

    await fireEvent.click(screen.getByRole('button', { name: 'Adicionar gêneros às tags' }));
    expect(api.editSeries).toHaveBeenCalledWith(series.id, { tags: ['action', 'Comedy'] }); // "Action" is already a tag
    await fireEvent.click(screen.getByRole('button', { name: /remover vínculo/ }));
    expect(api.unlinkSeries).toHaveBeenCalledWith(series.id);
    expect(onChange).toHaveBeenCalledTimes(2);
  });

  it('offers a lookup when never looked up, not found or unlinked', async () => {
    const series = makeSeries({ external: null });
    api.enrichSeries.mockResolvedValue({ ...series, external: anilist });
    const onChange = vi.fn();
    const first = render(ExternalInfo, { series, onChange });
    await fireEvent.click(screen.getByRole('button', { name: 'Buscar no AniList/MangaUpdates' }));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ external: anilist }));
    first.unmount();

    render(ExternalInfo, {
      series: makeSeries({ external: { ...anilist, id: '', source: '' } }),
      onChange,
    });
    expect(screen.getByText(/Não encontrada no AniList nem no MangaUpdates/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Buscar de novo' })).toBeInTheDocument();
  });
});

describe('SeriesAbout', () => {
  it('uses the external synopsis only when the site has none', () => {
    const { unmount } = render(SeriesAbout, {
      series: makeSeries({ synopsis: '', external: anilist }),
      onChange: vi.fn(),
    });
    expect(screen.getByText('Kaiden, um gato…')).toBeInTheDocument();
    expect(screen.getByText('Sinopse do AniList')).toBeInTheDocument();
    unmount();
    render(SeriesAbout, {
      series: makeSeries({ synopsis: 'Do site', external: anilist }),
      onChange: vi.fn(),
    });
    expect(screen.getByText('Do site')).toBeInTheDocument();
    expect(screen.queryByText('Kaiden, um gato…')).toBeNull();
  });
});
