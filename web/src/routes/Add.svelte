<script lang="ts">
  import PreviewForm from '../lib/components/PreviewForm.svelte';
  import Spinner from '../lib/components/Spinner.svelte';
  import { api, ApiError } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { t } from '../lib/i18n';
  import { href, router } from '../lib/stores/router.svelte';
  import type { Overrides, PreviewResult } from '../lib/api/types';

  // Add flow (RF-01, RF-02): URL → preview (editable) → save. A link shared
  // to the Android app (or opened as #/add?url=…) is read right away; shared
  // text may carry a title before the link.
  let { sharedUrl = '' }: { sharedUrl?: string } = $props();
  // svelte-ignore state_referenced_locally
  let url = $state(firstLink(sharedUrl));
  let preview = $state<PreviewResult | null>(null);
  let fetching = $state(false);
  let saving = $state(false);
  let error = $state('');
  // The site needs a recipe: read by the heuristic, or not read at all.
  let recipeHint = $state<'' | 'heuristic' | 'failed'>('');
  let canGenerate = $state(false);

  $effect(() => {
    api.health().then(
      (h) => (canGenerate = h.llm),
      () => (canGenerate = false),
    );
  });

  const unreadable = new Set(['unsupported_site', 'extraction_incomplete']);

  /** The host of the pasted URL without "www.", or "" if it is not a URL. */
  function site(): string {
    try {
      return new URL(url.trim()).hostname.replace(/^www\./, '');
    } catch {
      return '';
    }
  }

  /** The first http(s) link in text, or the text itself. */
  function firstLink(text: string): string {
    return text.match(/https?:\/\/\S+/)?.[0] ?? text;
  }

  $effect(() => {
    if (sharedUrl) void readPreview();
  });

  async function fetchPreview(e: SubmitEvent) {
    e.preventDefault();
    await readPreview();
  }

  async function readPreview() {
    fetching = true;
    error = '';
    preview = null;
    recipeHint = '';
    try {
      preview = await api.preview(url.trim());
      if (preview.method.startsWith('heuristic')) recipeHint = 'heuristic';
    } catch (err) {
      error = errorMessage(err);
      if (err instanceof ApiError && unreadable.has(err.code)) recipeHint = 'failed';
    } finally {
      fetching = false;
    }
  }

  async function save(overrides: Overrides) {
    saving = true;
    error = '';
    try {
      const s = await api.addSeries(url.trim(), overrides);
      router.go({ name: 'series', id: s.id });
    } catch (err) {
      error =
        err instanceof ApiError && err.code === 'duplicate'
          ? t('add.duplicate')
          : errorMessage(err);
    } finally {
      saving = false;
    }
  }
</script>

<section>
  <h1>{t('add.title')}</h1>
  <form class="url" onsubmit={fetchPreview}>
    <label for="url">{t('add.url')}</label>
    <div class="row">
      <input
        id="url"
        type="url"
        required
        placeholder="https://"
        bind:value={url}
        oninput={() => {
          preview = null;
          recipeHint = '';
        }}
      />
      <button type="submit" disabled={fetching}>
        {#if fetching}<Spinner size={14} />{/if}
        {t('add.fetch')}
      </button>
    </div>
    <p class="help">{t('add.url_help')}</p>
  </form>

  {#if fetching}<p class="busy"><Spinner label={t('add.fetching')} /></p>{/if}
  {#if error}<p class="error" role="alert">{error}</p>{/if}

  {#if recipeHint && site()}
    <aside class="hint">
      <p>{t(recipeHint === 'heuristic' ? 'add.recipe_hint' : 'add.recipe_failed')}</p>
      <a href={href({ name: 'recipes', domain: site(), url: url.trim() })}>
        {t(canGenerate ? 'add.recipe_generate' : 'add.recipe_create', { domain: site() })} →
      </a>
    </aside>
  {/if}

  {#if preview}
    <PreviewForm {preview} {saving} onSave={save} />
  {/if}
</section>

<style>
  section {
    max-width: 760px;
  }
  .row {
    display: flex;
    gap: 8px;
  }
  input {
    flex: 1;
    min-width: 0;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
  }
  button {
    padding: 10px 16px;
    border: 0;
    border-radius: 8px;
    background: var(--accent);
    color: var(--accent-text);
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.6;
  }
  .busy {
    margin: 12px 0;
  }
  button {
    display: inline-flex;
    align-items: center;
    gap: 8px;
  }
  .help {
    color: var(--text-muted);
    font-size: 0.85rem;
  }
  .error {
    color: var(--danger);
  }
  .hint {
    margin: 12px 0;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    font-size: 0.9rem;
  }
  .hint p {
    margin: 0 0 6px;
    color: var(--text-muted);
  }
</style>
