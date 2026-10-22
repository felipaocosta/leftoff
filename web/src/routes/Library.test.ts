import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { makeSeries } from '../lib/test-fixtures';

const listSeries = vi.fn();
const listSources = vi.fn();
const refreshSeries = vi.fn();
const refreshAll = vi.fn();
const job = vi.fn();
vi.mock('../lib/api', async (importOriginal) => {
  const orig = await importOriginal<typeof import('../lib/api')>();
  return {
    ...orig,
    api: {
      listSeries,
      listSources,
      refreshSeries,
      refreshAll,
      job,
      coverUrl: (id: string) => `c/${id}`,
    },
  };
});

const { default: Library } = await import('./Library.svelte');
const { prefs } = await import('../lib/stores/prefs.svelte');

describe('Library', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    prefs.setFiltersOpen(true);
    listSources.mockResolvedValue([]);
  });

  it('collapses the filters, counting the active ones', async () => {
    listSeries.mockResolvedValue([makeSeries()]);
    render(Library);
    const toggle = await screen.findByRole('button', { name: /Filtros/ });
    expect(toggle).toHaveAttribute('aria-expanded', 'true'); // wide screen by default
    await fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'ongoing' } });
    await fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(toggle).toHaveTextContent(/Filtros\s*1/);
    expect(screen.queryByRole('combobox', { name: 'Status' })).toBeNull(); // hidden panel
    expect(localStorage.getItem('mt.filters')).toBe('closed');
    await fireEvent.click(toggle);
    expect(screen.getByRole('combobox', { name: 'Status' })).toBeInTheDocument();
  });

  it('switches between grid and list and remembers the choice', async () => {
    listSeries.mockResolvedValue([makeSeries()]);
    const first = render(Library);
    const list = await screen.findByRole('button', { name: 'Lista' });
    expect(screen.getByRole('button', { name: 'Grade' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('heading', { name: 'Solo Leveling' })).toBeInTheDocument(); // card
    await fireEvent.click(list);
    expect(list).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByRole('heading', { name: 'Solo Leveling' })).toBeNull(); // row
    expect(screen.getByRole('link', { name: /Solo Leveling/ })).toBeInTheDocument();
    first.unmount();

    render(Library);
    expect(await screen.findByRole('button', { name: 'Lista' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await fireEvent.click(screen.getByRole('button', { name: 'Grade' }));
  });

  it('filters by source only when the library uses more than one site', async () => {
    listSeries.mockResolvedValue([makeSeries()]);
    listSources.mockResolvedValue([
      { domain: 'mangaread.org', count: 3 },
      { domain: 'ravenscans.org', count: 1 },
    ]);
    render(Library);
    const select = await screen.findByLabelText('Fonte');
    expect(screen.getByRole('option', { name: 'mangaread.org (3)' })).toBeInTheDocument();
    await fireEvent.change(select, { target: { value: 'ravenscans.org' } });
    await waitFor(() =>
      expect(listSeries).toHaveBeenLastCalledWith({
        q: '',
        sort: 'updated',
        source: 'ravenscans.org',
      }),
    );
  });

  it('hides the source filter for a single site or when sources fail', async () => {
    listSeries.mockResolvedValue([makeSeries()]);
    listSources.mockResolvedValueOnce([{ domain: 'mangaread.org', count: 1 }]);
    const first = render(Library);
    expect(await screen.findByText('1 obra')).toBeInTheDocument();
    expect(screen.queryByLabelText('Fonte')).toBeNull();
    first.unmount();

    listSources.mockRejectedValueOnce(new Error('down'));
    render(Library);
    expect(await screen.findByText('1 obra')).toBeInTheDocument();
    expect(screen.queryByLabelText('Fonte')).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('lists series and reloads with the selected filters', async () => {
    listSeries.mockResolvedValue([makeSeries(), makeSeries({ id: '2', title: 'Other' })]);
    render(Library);
    expect(await screen.findByText('2 obras')).toBeInTheDocument();
    expect(listSeries).toHaveBeenLastCalledWith({ q: '', sort: 'updated' });

    await fireEvent.change(screen.getByLabelText('Novidades'), { target: { value: 'true' } });
    await waitFor(() =>
      expect(listSeries).toHaveBeenLastCalledWith({ q: '', sort: 'updated', has_new: true }),
    );
  });

  it('shows the empty state with a link to add', async () => {
    listSeries.mockResolvedValue([]);
    render(Library);
    expect(await screen.findByText(/Sua biblioteca está vazia/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '+ Adicionar' })).toHaveAttribute('href', '#/add');
  });

  it('shows errors with a retry button', async () => {
    const { ApiError } = await import('../lib/api');
    listSeries.mockRejectedValueOnce(new ApiError(401, 'unauthorized', 'no'));
    listSeries.mockResolvedValue([]);
    render(Library);
    expect(await screen.findByRole('alert')).toHaveTextContent('Token inválido');
    await fireEvent.click(screen.getByRole('button', { name: 'Tentar de novo' }));
    expect(await screen.findByText(/Sua biblioteca está vazia/)).toBeInTheDocument();
  });

  it('runs refresh all as a server job and reports the result', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    listSeries.mockResolvedValue([makeSeries(), makeSeries({ id: '2' })]);
    const base = {
      id: '1',
      total: 2,
      failed: 1,
      new_chapters: 3,
      started_at: '',
      finished_at: null,
    };
    refreshAll.mockResolvedValue({ ...base, status: 'running', done: 0 });
    job
      .mockResolvedValueOnce({ ...base, status: 'running', done: 1 })
      .mockResolvedValue({ ...base, status: 'done', done: 2 });
    render(Library);
    await screen.findByText('2 obras');
    await fireEvent.click(screen.getByRole('button', { name: 'Atualizar tudo' }));
    expect(await screen.findByText('Atualizando 0 de 2…')).toBeInTheDocument();
    await vi.advanceTimersByTimeAsync(1100);
    expect(await screen.findByText('Atualizando 1 de 2…')).toBeInTheDocument();
    await vi.advanceTimersByTimeAsync(1100);
    expect(
      await screen.findByText('Atualização concluída: 3 capítulos novos, 1 falhas.'),
    ).toBeInTheDocument();
    expect(job).toHaveBeenCalledWith('1');
    vi.useRealTimers();
  });
});
