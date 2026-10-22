<script lang="ts">
  import RecipeEditor from '../lib/components/RecipeEditor.svelte';
  import RecipeHealth from '../lib/components/RecipeHealth.svelte';
  import Spinner from '../lib/components/Spinner.svelte';
  import { api } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { t } from '../lib/i18n';
  import { recipeTemplate } from '../lib/recipeTemplate';
  import type { RecipeInfo } from '../lib/api/types';

  // Recipes screen (RF-53): list, create, edit, test and remove. The Add
  // screen links here with a domain and URL to start that site's recipe.
  let { domain: wanted = '', url: wantedUrl = '' }: { domain?: string; url?: string } = $props();
  let recipes = $state<RecipeInfo[]>([]);
  let loading = $state(true);
  let error = $state('');
  let editing = $state<{ domain: string; json: string } | null>(null);
  let newDomain = $state('');
  let canGenerate = $state(false);

  async function load() {
    loading = true;
    try {
      recipes = await api.listRecipes();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  // Opens the requested site's recipe, or a new one from the template.
  async function openWanted() {
    await load();
    if (!wanted) return;
    if (recipes.some((r) => r.domain === wanted)) await open(wanted);
    else editing = { domain: wanted, json: recipeTemplate(wanted) };
  }

  $effect(() => {
    openWanted();
    api.health().then(
      (h) => (canGenerate = h.llm),
      () => (canGenerate = false), // the list shows connection errors
    );
  });

  async function open(domain: string) {
    error = '';
    try {
      editing = { domain, json: await api.getRecipe(domain) };
    } catch (err) {
      error = errorMessage(err);
    }
  }

  function create(e: SubmitEvent) {
    e.preventDefault();
    const domain = newDomain
      .trim()
      .toLowerCase()
      .replace(/^www\./, '');
    if (domain) editing = { domain, json: recipeTemplate(domain) };
    newDomain = '';
  }
</script>

<section>
  <h1>{t('recipes.title')}</h1>
  <p class="help">{t('recipes.help')}</p>
  {#if error}<p class="error" role="alert">{error}</p>{/if}

  <div class="layout">
    <div class="list">
      {#if loading}
        <Spinner label={t('app.loading')} />
      {:else if recipes.length === 0}
        <p class="help">{t('recipes.empty')}</p>
      {:else}
        <ul>
          {#each recipes as r (r.domain)}
            <li>
              <button
                type="button"
                class:active={editing?.domain === r.domain}
                onclick={() => open(r.domain)}
              >
                {r.domain}
                {#if r.error}<span class="bad" title={r.error}>{t('recipes.invalid')}</span>{/if}
                {#if r.failing?.length}
                  <span class="bad">{t('recipes.failing_badge', { n: r.failing.length })}</span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
      <form onsubmit={create}>
        <input
          placeholder={t('recipes.new_domain')}
          aria-label={t('recipes.new_domain')}
          bind:value={newDomain}
        />
        <button type="submit">{t('recipes.create')}</button>
      </form>
    </div>
    {#if editing}
      {@const failing = recipes.find((r) => r.domain === editing?.domain)?.failing ?? []}
      {#key editing.domain}
        <RecipeHealth domain={editing.domain} {failing} />
        <RecipeEditor
          domain={editing.domain}
          initial={editing.json}
          initialUrl={editing.domain === wanted ? wantedUrl : ''}
          {canGenerate}
          onSaved={load}
          onRemoved={() => {
            editing = null;
            load();
          }}
        />
      {/key}
    {/if}
  </div>
</section>

<style>
  h1 {
    font-size: 1.4rem;
    margin: 8px 0 4px;
  }
  .help {
    color: var(--text-muted);
    font-size: 0.9rem;
  }
  .error {
    color: var(--danger);
  }
  .layout {
    display: grid;
    grid-template-columns: minmax(180px, 240px) 1fr;
    gap: 20px;
    align-items: start;
  }
  @media (max-width: 640px) {
    .layout {
      grid-template-columns: 1fr;
    }
  }
  ul {
    list-style: none;
    padding: 0;
    margin: 0 0 12px;
  }
  li button {
    width: 100%;
    text-align: left;
    padding: 8px 10px;
    border: 0;
    border-radius: 8px;
    background: none;
    cursor: pointer;
  }
  li button.active,
  li button:hover {
    background: var(--surface-2);
  }
  .bad {
    color: var(--warning);
    font-size: 0.75rem;
    margin-left: 6px;
  }
  form {
    display: flex;
    gap: 6px;
  }
  form input {
    flex: 1;
    min-width: 0;
    padding: 6px 8px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
  }
  form button {
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    cursor: pointer;
  }
</style>
