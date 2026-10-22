import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { makeSeries } from '../lib/test-fixtures';
import type { PreviewResult } from '../lib/api/types';

const api = { preview: vi.fn(), addSeries: vi.fn(), health: vi.fn() };
vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));
const { default: Add } = await import('./Add.svelte');
const { ApiError } = await import('../lib/api');

const URL = 'https://site.org/manga/solo';

function preview(overrides: Partial<PreviewResult> = {}): PreviewResult {
  return {
    series: {
      url: URL,
      title: 'Solo Leveling',
      alt_titles: [],
      cover_url: 'https://site.org/c.jpg',
      status: 'ongoing',
      status_raw: 'OnGoing',
      synopsis: '',
      type: 'manhwa',
    },
    chapters: [
      { number: 2, label: 'Chapter 2', url: URL + '/2', alt_urls: null, released_at: null },
      { number: 1, label: 'Chapter 1', url: URL + '/1', alt_urls: null, released_at: null },
    ],
    method: 'adapter:test',
    confidence: 1,
    needs_confirmation: false,
    ...overrides,
  };
}

async function fetchPreview() {
  render(Add);
  await fireEvent.input(screen.getByLabelText('Link da obra'), { target: { value: URL } });
  await fireEvent.click(screen.getByRole('button', { name: 'Buscar' }));
}

describe('Add', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    api.health.mockResolvedValue({ status: 'ok', version: 't', mode: 'server', llm: false });
  });

  it('shows a spinner while the site is being read', async () => {
    let resolve: (p: PreviewResult) => void = () => {};
    api.preview.mockReturnValue(new Promise((r) => (resolve = r)));
    await fetchPreview();
    expect(screen.getByRole('status')).toHaveTextContent('Buscando dados no site…');
    expect(screen.getByRole('button', { name: 'Buscar' })).toBeDisabled();
    resolve(preview());
    expect(await screen.findByText('2 capítulos encontrados · último: 2')).toBeInTheDocument();
    expect(screen.queryByText('Buscando dados no site…')).not.toBeInTheDocument();
  });

  it('previews, sends only the changed fields and opens the new series', async () => {
    api.preview.mockResolvedValue(preview());
    api.addSeries.mockResolvedValue(makeSeries({ id: 'NEW' }));
    await fetchPreview();
    expect(api.preview).toHaveBeenCalledWith(URL);
    expect(await screen.findByText('2 capítulos encontrados · último: 2')).toBeInTheDocument();
    expect(screen.getByText('Extraído por adapter:test · confiança 100%')).toBeInTheDocument();

    await fireEvent.input(screen.getByLabelText('Título'), {
      target: { value: 'Solo Leveling (BR)' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Salvar na biblioteca' }));
    expect(api.addSeries).toHaveBeenCalledWith(URL, {
      category: 'reading',
      title: 'Solo Leveling (BR)',
    });
    await waitFor(() => expect(window.location.hash).toBe('#/series/NEW'));
  });

  it('requires confirmation when the confidence is low', async () => {
    api.preview.mockResolvedValue(preview({ confidence: 0.4, needs_confirmation: true }));
    await fetchPreview();
    const save = await screen.findByRole('button', { name: 'Salvar na biblioteca' });
    expect(screen.getByText(/confiança da extração está baixa/)).toBeInTheDocument();
    expect(save).toBeDisabled();
    await fireEvent.click(screen.getByLabelText(/Conferi os dados/));
    expect(save).toBeEnabled();
  });

  it('explains duplicates and extraction errors', async () => {
    api.preview.mockRejectedValueOnce(new ApiError(422, 'unsupported_site', 'unsupported site'));
    await fetchPreview();
    expect(await screen.findByRole('alert')).toHaveTextContent('unsupported site');

    api.preview.mockResolvedValue(preview());
    api.addSeries.mockRejectedValue(new ApiError(409, 'duplicate', 'exists'));
    await fireEvent.click(screen.getByRole('button', { name: 'Buscar' }));
    await fireEvent.click(await screen.findByRole('button', { name: 'Salvar na biblioteca' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Essa obra já está na sua biblioteca.',
    );
  });

  it('suggests a recipe when the heuristic read the site or it could not be read', async () => {
    api.health.mockResolvedValue({ status: 'ok', version: 't', mode: 'server', llm: true });
    api.preview
      .mockResolvedValueOnce(preview({ method: 'heuristic+browser' }))
      .mockResolvedValueOnce(preview())
      .mockRejectedValueOnce(new ApiError(422, 'unsupported_site', 'no'));
    render(Add);
    const input = screen.getByLabelText('Link da obra');
    await fireEvent.input(input, { target: { value: 'https://www.site.org/manga/solo' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Buscar' }));
    const link = await screen.findByRole('link', {
      name: /Gerar uma receita com IA para site\.org/,
    });
    expect(screen.getByText(/adivinhados pela heurística/)).toBeInTheDocument();
    expect(link.getAttribute('href')).toBe(
      '#/recipes?domain=site.org&url=https%3A%2F%2Fwww.site.org%2Fmanga%2Fsolo',
    );

    await fireEvent.click(screen.getByRole('button', { name: 'Buscar' })); // read by an adapter
    await waitFor(() => expect(screen.queryByRole('link', { name: /receita/ })).toBeNull());

    await fireEvent.click(screen.getByRole('button', { name: 'Buscar' })); // not readable
    expect(await screen.findByText(/não conseguiu ler este site/)).toBeInTheDocument();
  });

  it('reads a shared link right away, even with text before it', async () => {
    api.preview.mockResolvedValue(preview());
    render(Add, { sharedUrl: 'Solo Leveling - leia online https://site.org/manga/solo' });
    expect(await screen.findByDisplayValue('Solo Leveling')).toBeInTheDocument(); // the preview form
    expect(api.preview).toHaveBeenCalledWith('https://site.org/manga/solo');
    expect((screen.getByLabelText('Link da obra') as HTMLInputElement).value).toBe(
      'https://site.org/manga/solo',
    );
  });
});
