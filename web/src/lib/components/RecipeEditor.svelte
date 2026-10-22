<script lang="ts">
  import RecipeTestResult from './RecipeTestResult.svelte';
  import Spinner from './Spinner.svelte';
  import { api } from '../api';
  import { errorMessage } from '../errors';
  import { t } from '../i18n';
  import type { RecipeTest } from '../api/types';

  // Edits one recipe's JSON, tests it on a page, generates it with the
  // language model (canGenerate) and saves or removes it.
  let {
    domain,
    initial,
    initialUrl = '',
    canGenerate = false,
    onSaved,
    onRemoved,
  }: {
    domain: string;
    initial: string;
    initialUrl?: string;
    canGenerate?: boolean;
    onSaved: () => void;
    onRemoved: () => void;
  } = $props();

  // The editor starts from the stored text; later prop changes are not wanted here.
  // svelte-ignore state_referenced_locally
  let json = $state(initial);
  // svelte-ignore state_referenced_locally
  let url = $state(initialUrl);
  let busy = $state(false);
  let generating = $state(false);
  let error = $state('');
  let notice = $state('');
  let result = $state<RecipeTest | null>(null);
  let notes = $state<string[]>([]);

  function parsed(): boolean {
    try {
      JSON.parse(json);
      return true;
    } catch (err) {
      error = t('recipes.bad_json', { message: (err as Error).message });
      return false;
    }
  }

  async function run(action: () => Promise<void>, needsJSON = true) {
    error = notice = '';
    if (needsJSON && !parsed()) return;
    busy = true;
    try {
      await action();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const test = () =>
    run(async () => {
      result = await api.testRecipe(domain, json, url.trim());
      notes = [];
    });
  const generate = () =>
    run(async () => {
      generating = true;
      try {
        const gen = await api.generateRecipe(domain, url.trim());
        json = gen.recipe;
        result = { result: gen.result, problems: gen.problems };
        notes = gen.notes;
        notice = t('recipes.generated', { attempts: gen.attempts });
      } finally {
        generating = false;
      }
    }, false);
  const save = () =>
    run(async () => {
      await api.saveRecipe(domain, json);
      notice = t('recipes.saved');
      onSaved();
    });

  async function remove() {
    if (!window.confirm(t('recipes.remove_confirm', { domain }))) return;
    try {
      await api.deleteRecipe(domain);
      onRemoved();
    } catch (err) {
      error = errorMessage(err);
    }
  }
</script>

<section>
  <h2>{domain}</h2>
  <label>
    {t('recipes.json')}
    <textarea rows="18" spellcheck="false" bind:value={json}></textarea>
  </label>
  <div class="row">
    <input type="url" placeholder="https://" aria-label={t('recipes.test_url')} bind:value={url} />
    <button type="button" onclick={test} disabled={busy || !url.trim()}>
      {#if busy && !generating}<Spinner size={14} />{/if}
      {busy && !generating ? t('recipes.testing') : t('recipes.test')}
    </button>
    {#if canGenerate}
      <button type="button" onclick={generate} disabled={busy || !url.trim()}>
        {#if generating}<Spinner size={14} />{/if}
        {generating ? t('recipes.generating') : t('recipes.generate')}
      </button>
    {/if}
  </div>
  {#if canGenerate}<p class="help">{t('recipes.generate_help')}</p>{/if}
  <div class="row">
    <button type="button" class="primary" onclick={save} disabled={busy}>{t('recipes.save')}</button
    >
    <button type="button" class="danger" onclick={remove} disabled={busy}
      >{t('recipes.remove')}</button
    >
  </div>
  {#if error}<p class="error" role="alert">{error}</p>{/if}
  {#if notice}<p class="notice" role="status">{notice}</p>{/if}
  {#if result}<RecipeTestResult test={result} {notes} />{/if}
</section>

<style>
  section {
    display: grid;
    gap: 10px;
  }
  h2 {
    margin: 0;
    font-size: 1.1rem;
  }
  label {
    display: grid;
    gap: 4px;
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  textarea {
    font-family: ui-monospace, monospace;
    font-size: 0.8rem;
    padding: 8px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    color: var(--text);
  }
  .row {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  input {
    flex: 1;
    min-width: 0;
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
  }
  button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 14px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    cursor: pointer;
  }
  .primary {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-text);
  }
  .danger {
    color: var(--danger);
  }
  .error {
    color: var(--danger);
  }
  .notice {
    color: var(--success);
  }
  .help {
    margin: 0;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
</style>
