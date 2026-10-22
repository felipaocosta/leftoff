import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import SeriesRow from './SeriesRow.svelte';
import { makeSeries } from '../test-fixtures';

describe('SeriesRow', () => {
  it('shows the card information on one row, linking to the series', () => {
    render(SeriesRow, {
      series: makeSeries({
        has_new: true,
        check_error: {
          code: 'blocked',
          message: 'blocked',
          at: '2026-10-19T12:00:00Z',
          failures: 1,
        },
      }),
    });
    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', '#/series/01TEST');
    expect(link).toHaveTextContent('Solo Leveling');
    expect(link).toHaveTextContent('Em andamento');
    expect(link).toHaveTextContent('Cap. 100 / 120');
    expect(screen.getByText('20 não lidos')).toBeInTheDocument();
    expect(screen.getByText('Novo')).toBeInTheDocument();
    expect(screen.getByText('Erro na última verificação')).toBeInTheDocument();
  });

  it('hides empty badges', () => {
    render(SeriesRow, { series: makeSeries({ unread_count: 0 }) });
    expect(screen.queryByText(/não lido/)).toBeNull();
    expect(screen.queryByText('Novo')).toBeNull();
  });
});
