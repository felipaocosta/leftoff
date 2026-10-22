import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { makeSeries } from '../lib/test-fixtures';

const api = { listSeries: vi.fn(), markSeen: vi.fn(), coverUrl: (id: string) => `c/${id}` };
vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));
const { default: News } = await import('./News.svelte');
const { news } = await import('../lib/stores/news.svelte');

describe('News', () => {
  beforeEach(() => vi.resetAllMocks());

  it('lists series with news, updates the badge and marks all as seen', async () => {
    api.listSeries
      .mockResolvedValueOnce([
        makeSeries({ has_new: true }),
        makeSeries({ id: '2', title: 'Other', has_new: true }),
      ])
      .mockResolvedValue([]);
    api.markSeen.mockResolvedValue(makeSeries());
    render(News);
    expect(await screen.findByRole('heading', { name: 'Other' })).toBeInTheDocument();
    expect(api.listSeries).toHaveBeenCalledWith({ has_new: true, sort: 'updated' });
    expect(news.count).toBe(2);

    await fireEvent.click(screen.getByRole('button', { name: 'Marcar todas como vistas' }));
    expect(await screen.findByText(/Nenhuma novidade/)).toBeInTheDocument();
    expect(api.markSeen).toHaveBeenCalledTimes(2);
    expect(news.count).toBe(0);
  });
});
