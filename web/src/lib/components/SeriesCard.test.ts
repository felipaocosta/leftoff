import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import SeriesCard from './SeriesCard.svelte';
import { makeSeries } from '../test-fixtures';

describe('SeriesCard', () => {
  it('shows title, status, progress and unread badge, linking to the series', () => {
    render(SeriesCard, { series: makeSeries() });
    expect(screen.getByRole('heading', { name: 'Solo Leveling' })).toBeInTheDocument();
    expect(screen.getByText('Em andamento')).toBeInTheDocument();
    expect(screen.getByText('Cap. 100 / 120')).toBeInTheDocument();
    expect(screen.getByText('20 não lidos')).toBeInTheDocument();
    expect(screen.queryByText('Novo')).not.toBeInTheDocument();
    expect(screen.getByRole('link')).toHaveAttribute('href', '#/series/01TEST');
  });

  it('shows the new and error badges', () => {
    render(SeriesCard, {
      series: makeSeries({
        has_new: true,
        unread_count: 0,
        last_read_chapter: null,
        check_error: {
          code: 'blocked',
          message: 'blocked',
          at: '2026-10-19T12:00:00Z',
          failures: 1,
        },
      }),
    });
    expect(screen.getByText('Novo')).toBeInTheDocument();
    expect(screen.getByText('Erro na última verificação')).toBeInTheDocument();
    expect(screen.getByText('Cap. — / 120')).toBeInTheDocument();
    expect(screen.queryByText(/não lidos/)).not.toBeInTheDocument();
  });

  it('versions the cover URL with cover.updated_at, so a new cover shows on the library', () => {
    const cover = {
      remote_url: 'r',
      local: 'covers/01TEST.png',
      custom: true,
      updated_at: '2026-10-20T03:00:00Z',
    };
    render(SeriesCard, { series: makeSeries({ cover }) });
    expect(screen.getByRole('img', { name: 'Solo Leveling' })).toHaveAttribute(
      'src',
      'api/v1/covers/01TEST?v=' + Date.parse('2026-10-20T03:00:00Z'),
    );
  });

  it('falls back from the local cover to the remote one and then to a placeholder', async () => {
    render(SeriesCard, { series: makeSeries() });
    const img = screen.getByRole('img', { name: 'Solo Leveling' });
    expect(img).toHaveAttribute('src', 'api/v1/covers/01TEST');
    await fireEvent.error(img);
    expect(screen.getByRole('img', { name: 'Solo Leveling' })).toHaveAttribute(
      'src',
      'https://site.org/c.jpg',
    );
    await fireEvent.error(screen.getByRole('img', { name: 'Solo Leveling' }));
    expect(screen.getByRole('img', { name: 'Sem capa' })).toBeInTheDocument();
  });
});
