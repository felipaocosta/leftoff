<script lang="ts">
  import SeriesCard from '../lib/components/SeriesCard.svelte';
  import SeriesRow from '../lib/components/SeriesRow.svelte';
  import ViewToggle from '../lib/components/ViewToggle.svelte';
  import Spinner from '../lib/components/Spinner.svelte';
  import LibraryToolbar from '../lib/components/LibraryToolbar.svelte';
  import { api } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { onPullRefresh } from '../lib/pullRefresh';
  import { t } from '../lib/i18n';
  import { news } from '../lib/stores/news.svelte';
  import { prefs } from '../lib/stores/prefs.svelte';
  import { refreshJob } from '../lib/stores/refreshJob.svelte';
  import { href } from '../lib/stores/router.svelte';
  import type { Series, SeriesQuery, SourceCount } from '../lib/api/types';

  let query = $state<SeriesQuery>({ q: '', sort: 'updated' });
  let series = $state<Series[]>([]);
  let sources = $state<SourceCount[]>([]);
  let loading = $state(true);
  let error = $state('');
  let summary = $state('');

  let filtered = $derived(
    Boolean(query.q || query.status || query.category || query.type || query.source) ||
      query.has_new !== undefined,
  );

  async function load(q: SeriesQuery) {
    loading = true;
    error = '';
    try {
      series = await api.listSeries(q);
      api.listSources().then(
        (list) => (sources = list),
        () => (sources = []), // optional: the filter just hides
      );
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  // Reload on every filter change; typing in the search box is debounced.
  $effect(() => {
    const q = { ...query };
    const timer = setTimeout(() => load(q), q.q ? 250 : 0);
    return () => clearTimeout(timer);
  });

  // RF-21: "refresh all" runs as a server job; progress survives navigation.
  let progress = $derived(refreshJob.running ? refreshJob.job : null);

  $effect(() =>
    refreshJob.onDone((job) => {
      summary = t('library.refresh_done', { new: job.new_chapters, failed: job.failed });
      load({ ...query });
      news.refresh();
    }),
  );

  $effect(() => {
    if (refreshJob.error) error = errorMessage(refreshJob.error);
  });

  function refreshAll() {
    summary = '';
    refreshJob.start();
  }

  // Android: pulling the library down starts "refresh all"; its progress
  // shows below the toolbar, so the pull spinner stops right away.
  $effect(() =>
    onPullRefresh(() => {
      if (!progress) refreshAll();
    }),
  );
</script>

<section>
  <div class="head">
    <h1>{t('nav.library')}</h1>
    <button type="button" onclick={refreshAll} disabled={progress !== null}>
      {#if progress}<Spinner size={14} />{/if}
      {t('library.refresh_all')}
    </button>
  </div>

  <LibraryToolbar bind:query {sources} />

  <p class="status" aria-live="polite">
    {#if progress}
      <Spinner
        size={14}
        label={t('library.refreshing', { done: progress.done, total: progress.total })}
      />
    {:else if summary}
      {summary}
    {/if}
  </p>

  {#if error}
    <div class="error" role="alert">
      <p>{error}</p>
      <button type="button" onclick={() => load({ ...query })}>{t('app.retry')}</button>
    </div>
  {:else if loading && series.length === 0}
    <p class="loading"><Spinner label={t('app.loading')} /></p>
  {:else if series.length === 0}
    <p class="empty">
      {#if filtered}
        {t('library.no_results')}
      {:else}
        {t('library.empty')} <a href={href({ name: 'add' })}>{t('nav.add')}</a>
      {/if}
    </p>
  {:else}
    <div class="list-head">
      <p class="count">{t('library.count', { n: series.length })}</p>
      <ViewToggle />
    </div>
    {#if prefs.view === 'list'}
      <ul class="rows">
        {#each series as s (s.id)}
          <li><SeriesRow series={s} /></li>
        {/each}
      </ul>
    {:else}
      <ul class="grid">
        {#each series as s (s.id)}
          <li><SeriesCard series={s} /></li>
        {/each}
      </ul>
    {/if}
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
    margin: 8px 0 12px;
  }
  .grid {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 14px;
  }
  .rows {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 6px;
  }
  .list-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }
  .list-head .count {
    margin: 0;
  }
  .count,
  .status {
    color: var(--text-muted);
    font-size: 0.85rem;
    margin: 0 0 8px;
    min-height: 1em;
  }
  .error {
    color: var(--danger);
  }
  .loading {
    padding: 24px 0;
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
  button:disabled {
    opacity: 0.6;
    cursor: progress;
  }
</style>
