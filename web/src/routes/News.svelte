<script lang="ts">
  import SeriesCard from '../lib/components/SeriesCard.svelte';
  import Spinner from '../lib/components/Spinner.svelte';
  import { api } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { t } from '../lib/i18n';
  import { news } from '../lib/stores/news.svelte';
  import type { Series } from '../lib/api/types';

  // Series with unseen chapters (RF-23), most recently updated first.
  let series = $state<Series[]>([]);
  let loading = $state(true);
  let busy = $state(false);
  let error = $state('');

  async function load() {
    loading = true;
    error = '';
    try {
      series = await api.listSeries({ has_new: true, sort: 'updated' });
      news.count = series.length;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    load();
  });

  async function seenAll() {
    busy = true;
    try {
      for (const s of series) await api.markSeen(s.id);
      await load();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<section>
  <div class="head">
    <h1>{t('news.title')}</h1>
    {#if series.length > 0}
      <button type="button" onclick={seenAll} disabled={busy}>
        {#if busy}<Spinner size={14} />{/if}
        {t('news.seen_all')}
      </button>
    {/if}
  </div>
  <p class="help">{t('news.help')}</p>

  {#if error}
    <p class="error" role="alert">{error}</p>
  {:else if loading}
    <p><Spinner label={t('app.loading')} /></p>
  {:else if series.length === 0}
    <p class="help">{t('news.empty')}</p>
  {:else}
    <ul class="grid">
      {#each series as s (s.id)}
        <li><SeriesCard series={s} /></li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
  }
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
  .grid {
    list-style: none;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 14px;
  }
  button {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 8px 14px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    cursor: pointer;
  }
</style>
