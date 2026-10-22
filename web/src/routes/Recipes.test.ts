import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../lib/api/client';
import type { RecipeTest } from '../lib/api/types';

const api = {
  listRecipes: vi.fn(),
  getRecipe: vi.fn(),
  saveRecipe: vi.fn(),
  deleteRecipe: vi.fn(),
  testRecipe: vi.fn(),
  generateRecipe: vi.fn(),
  health: vi.fn(),
};
vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));
const { default: Recipes } = await import('./Recipes.svelte');

const okTest: RecipeTest = {
  problems: [],
  result: {
    series: {
      url: 'u',
      title: 'Solo',
      alt_titles: [],
      cover_url: '',
      status: 'ongoing',
      status_raw: 'Ongoing',
      synopsis: '',
      type: 'other',
    },
    chapters: [
      { number: 2, label: 'Chapter 2', url: 'u/2', alt_urls: null, released_at: '2026-10-01' },
    ],
    method: 'recipe:site.org',
    confidence: 1,
    needs_confirmation: false,
  },
};

describe('Recipes', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    api.health.mockResolvedValue({ status: 'ok', version: 't', mode: 'server', llm: false });
  });

  it('creates a recipe from the template, tests it and saves it', async () => {
    api.listRecipes
      .mockResolvedValueOnce([])
      .mockResolvedValue([
        { domain: 'site.org', aliases: [], generated_by: 'manual', updated_at: '' },
      ]);
    api.testRecipe.mockResolvedValue(okTest);
    api.saveRecipe.mockResolvedValue(undefined);
    render(Recipes);
    expect(await screen.findByText('Nenhuma receita ainda.')).toBeInTheDocument();

    await fireEvent.input(screen.getByLabelText('Domínio do site (ex.: site.org)'), {
      target: { value: 'www.Site.org' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Criar' }));
    expect(screen.getByRole('heading', { name: 'site.org' })).toBeInTheDocument();
    expect((screen.getByLabelText('Receita (JSON)') as HTMLTextAreaElement).value).toContain(
      '"domain": "site.org"',
    );

    await fireEvent.input(screen.getByLabelText('Link de uma obra para testar'), {
      target: { value: 'https://site.org/manga/solo' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Testar' }));
    expect(await screen.findByText('A receita funcionou nesta página.')).toBeInTheDocument();
    expect(api.testRecipe).toHaveBeenCalledWith(
      'site.org',
      expect.stringContaining('"schema": 1'),
      'https://site.org/manga/solo',
    );

    await fireEvent.click(screen.getByRole('button', { name: 'Salvar' }));
    expect(await screen.findByText('Receita salva.')).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'site.org' })).toBeInTheDocument(),
    );
  });

  it('shows invalid JSON without calling the server, and invalid stored recipes', async () => {
    api.listRecipes.mockResolvedValue([
      {
        domain: 'broken.org',
        aliases: [],
        generated_by: '',
        updated_at: '',
        error: 'invalid recipe: schema must be 1',
      },
    ]);
    api.getRecipe.mockResolvedValue('{"schema": 2');
    render(Recipes);
    expect(await screen.findByText('inválida')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: /broken\.org/ }));
    await fireEvent.click(await screen.findByRole('button', { name: 'Salvar' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('O JSON da receita é inválido');
    expect(api.saveRecipe).not.toHaveBeenCalled();
  });

  it('offers "Gerar com IA" only with a model and fills the editor with the result', async () => {
    api.listRecipes.mockResolvedValue([]);
    const first = render(Recipes);
    await fireEvent.input(await screen.findByLabelText('Domínio do site (ex.: site.org)'), {
      target: { value: 'site.org' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Criar' }));
    expect(screen.queryByRole('button', { name: 'Gerar com IA' })).not.toBeInTheDocument();
    first.unmount();

    api.health.mockResolvedValue({ status: 'ok', version: 't', mode: 'server', llm: true });
    api.generateRecipe
      .mockResolvedValueOnce({
        ...okTest,
        recipe: '{"domain": "site.org", "x": 1}',
        notes: ['no chapter has a date'],
        attempts: 2,
      })
      .mockRejectedValueOnce(new ApiError(422, 'no_recipe', 'no'));
    render(Recipes);
    await fireEvent.input(await screen.findByLabelText('Domínio do site (ex.: site.org)'), {
      target: { value: 'site.org' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Criar' }));
    await fireEvent.input(screen.getByLabelText('Link de uma obra para testar'), {
      target: { value: ' https://site.org/manga/solo ' },
    });
    const generate = await screen.findByRole('button', { name: 'Gerar com IA' });
    await fireEvent.click(generate);
    expect(await screen.findByText(/Receita gerada pela IA \(2 tentativa/)).toBeInTheDocument();
    expect(api.generateRecipe).toHaveBeenCalledWith('site.org', 'https://site.org/manga/solo');
    expect((screen.getByLabelText('Receita (JSON)') as HTMLTextAreaElement).value).toBe(
      '{"domain": "site.org", "x": 1}',
    );
    expect(screen.getByText('A receita funcionou nesta página.')).toBeInTheDocument();
    expect(api.saveRecipe).not.toHaveBeenCalled();
    expect(screen.getByText('no chapter has a date')).toBeInTheDocument();

    await fireEvent.click(generate);
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'A IA não conseguiu escrever uma receita válida',
    );
  });

  it('opens the site requested by the Add screen with its test URL', async () => {
    api.listRecipes.mockResolvedValue([
      { domain: 'known.org', aliases: [], generated_by: 'manual', updated_at: '' },
    ]);
    api.getRecipe.mockResolvedValue('{"domain": "known.org"}');
    const first = render(Recipes, { domain: 'new.org', url: 'https://new.org/m/1' });
    expect(await screen.findByRole('heading', { name: 'new.org' })).toBeInTheDocument();
    expect((screen.getByLabelText('Receita (JSON)') as HTMLTextAreaElement).value).toContain(
      '"domain": "new.org"',
    );
    expect((screen.getByLabelText('Link de uma obra para testar') as HTMLInputElement).value).toBe(
      'https://new.org/m/1',
    );
    first.unmount();

    render(Recipes, { domain: 'known.org' });
    await waitFor(() =>
      expect((screen.getByLabelText('Receita (JSON)') as HTMLTextAreaElement).value).toBe(
        '{"domain": "known.org"}',
      ),
    );
  });

  it('marks failing recipes and links each failing series to regenerate', async () => {
    api.listRecipes.mockResolvedValue([
      {
        domain: 'site.org',
        aliases: [],
        generated_by: 'llm:x',
        updated_at: '',
        failing: [
          { series_id: '1', title: 'Solo', url: 'https://site.org/m/solo', reason: 'fallback' },
        ],
      },
    ]);
    api.getRecipe.mockResolvedValue('{"domain": "site.org"}');
    render(Recipes);
    await fireEvent.click(await screen.findByRole('button', { name: /site\.org/ }));
    expect(screen.getByText('falhando (1)')).toBeInTheDocument();
    expect(await screen.findByText(/parou de funcionar/)).toBeInTheDocument();
    expect(screen.getByText(/lida pela heurística/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'usar este link' })).toHaveAttribute(
      'href',
      '#/recipes?domain=site.org&url=https%3A%2F%2Fsite.org%2Fm%2Fsolo',
    );
  });
});
