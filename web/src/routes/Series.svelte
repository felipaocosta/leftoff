<script lang="ts">
  import ChapterList from '../lib/components/ChapterList.svelte';
  import EditSeries from '../lib/components/EditSeries.svelte';
  import SeriesAbout from '../lib/components/SeriesAbout.svelte';
  import SeriesHeader from '../lib/components/SeriesHeader.svelte';
  import Spinner from '../lib/components/Spinner.svelte';
  import { api, ApiError } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { onPullRefresh } from '../lib/pullRefresh';
  import { t } from '../lib/i18n';
  import { withRead, withReadUpTo } from '../lib/series';
  import { news } from '../lib/stores/news.svelte';
  import { href, router } from '../lib/stores/router.svelte';
  import type { Chapter, Patch, Series } from '../lib/api/types';

  // Series page (RF-35): loads the series, clears its "new" flag (RF-23) and
  // applies read changes optimistically, rolling back on failure.
  let { id }: { id: string } = $props();

  let series = $state<Series | null>(null);
  let error = $state('');
  let notice = $state('');
  let editing = $state(false);
  let refreshing = $state(false);
  let coverBusy = $state(false);

  $effect(() => {
    load(id);
  });

  async function load(seriesId: string) {
    error = '';
    try {
      let s = await api.getSeries(seriesId);
      if (s.has_new) {
        s = { ...(await api.markSeen(seriesId)), chapters: s.chapters };
        news.refresh();
      }
      series = s;
    } catch (err) {
      error = errorMessage(err);
    }
  }

  async function optimistic(next: Series, call: () => Promise<Series>) {
    const previous = series;
    series = next;
    try {
      series = await call();
    } catch (err) {
      series = previous;
      error = errorMessage(err);
    }
  }

  function toggle(c: Chapter, read: boolean) {
    if (series) optimistic(withRead(series, [c.key], read), () => api.markRead(id, [c.key], read));
  }

  function upTo(c: Chapter) {
    if (series && c.number !== null) {
      const n = c.number;
      optimistic(withReadUpTo(series, n), () => api.markReadUpTo(id, n));
    }
  }

  async function refresh() {
    refreshing = true;
    error = notice = '';
    try {
      const res = await api.refreshSeries(id);
      series = res.series;
      notice = t('series.refreshed', { n: res.new_chapters });
    } catch (err) {
      error = errorMessage(err);
      await load(id); // the failure is now in check_error
    } finally {
      refreshing = false;
    }
  }

  // Android: pulling the page down refreshes the series.
  $effect(() => onPullRefresh(() => (refreshing ? undefined : refresh())));

  // Cover actions return the series with a new cover.updated_at, which
  // changes the image URL everywhere.
  async function coverAction(call: () => Promise<Series>, done: string) {
    coverBusy = true;
    error = notice = '';
    try {
      series = { ...(await call()), chapters: series?.chapters ?? null };
      notice = done;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      coverBusy = false;
    }
  }

  // Saves the edits; a new address moves the series first (RF-08), asking
  // before moving to what looks like another work.
  async function save(patch: Patch, sourceUrl: string) {
    error = '';
    try {
      if (series && sourceUrl && sourceUrl !== series.source.url) {
        await moveTo(sourceUrl);
      }
      series = await api.editSeries(id, patch);
      editing = false;
    } catch (err) {
      error = errorMessage(err);
    }
  }

  async function moveTo(url: string) {
    try {
      await api.changeSource(id, url);
    } catch (err) {
      if (!(err instanceof ApiError) || err.code !== 'different_series') throw err;
      const title = /"([^"]*)"/.exec(err.message)?.[1] ?? url;
      if (!window.confirm(t('series.source_different', { title }))) throw err;
      await api.changeSource(id, url, true);
    }
  }

  async function remove() {
    if (!series || !window.confirm(t('series.remove_confirm', { title: series.title }))) return;
    try {
      await api.removeSeries(id);
      router.go({ name: 'library' });
    } catch (err) {
      error = errorMessage(err);
    }
  }
</script>

<a class="back" href={href({ name: 'library' })}>{t('series.back')}</a>

{#if error}<p class="error" role="alert">{error}</p>{/if}
{#if notice}<p class="notice" role="status">{notice}</p>{/if}

{#if series}
  {#if editing}
    <EditSeries {series} onSave={save} onCancel={() => (editing = false)} />
  {:else}
    <SeriesHeader
      {series}
      {refreshing}
      {coverBusy}
      onRefresh={refresh}
      onRefreshCover={() => coverAction(() => api.refreshCover(id), t('series.cover_refreshed'))}
      onUploadCover={(file) =>
        coverAction(() => api.uploadCover(id, file), t('series.cover_uploaded'))}
      onEdit={() => (editing = true)}
      onRemove={remove}
    />
  {/if}

  <SeriesAbout {series} onChange={(s) => (series = s)} />
  {#if series.notes}<p class="notes">{series.notes}</p>{/if}

  <h2>{t('series.chapters', { n: series.chapters?.length ?? 0 })}</h2>
  <ChapterList
    chapters={series.chapters ?? []}
    fallbackUrl={series.source.url}
    onToggle={toggle}
    onUpTo={upTo}
  />
{:else if !error}
  <p class="loading"><Spinner label={t('app.loading')} /></p>
{/if}

<style>
  .loading {
    padding: 24px 0;
  }
  .back {
    display: inline-block;
    margin-bottom: 12px;
    text-decoration: none;
  }
  h2 {
    font-size: 1.1rem;
    margin: 24px 0 8px;
  }
  .notes {
    white-space: pre-line;
    max-width: 75ch;
  }
  .notes {
    color: var(--text-muted);
    font-style: italic;
  }
  .error {
    color: var(--danger);
  }
  .notice {
    color: var(--success);
  }
</style>
