<script lang="ts">
  import CheckErrorNotice from './CheckErrorNotice.svelte';
  import ContinueButton from './ContinueButton.svelte';
  import Cover from './Cover.svelte';
  import CoverActions from './CoverActions.svelte';
  import Spinner from './Spinner.svelte';
  import {
    formatChapter,
    formatDateTime,
    frequencyLabel,
    releaseLabel,
    releaseState,
  } from '../format';
  import { t } from '../i18n';
  import type { Series } from '../api/types';

  // Top of the series page: cover, metadata, release estimate and actions.
  let {
    series,
    refreshing,
    coverBusy,
    onRefresh,
    onEdit,
    onRemove,
    onRefreshCover,
    onUploadCover,
  }: {
    series: Series;
    refreshing: boolean;
    coverBusy: boolean;
    onRefresh: () => void;
    onEdit: () => void;
    onRemove: () => void;
    onRefreshCover: () => void;
    onUploadCover: (file: File) => void;
  } = $props();

  let release = $derived(releaseLabel(series.next_release_estimate, new Date()));
  let overdue = $derived(releaseState(series.next_release_estimate, new Date()).kind === 'overdue');
  let checked = $derived(formatDateTime(series.last_checked_at));
</script>

<div class="header">
  <div>
    <div class="cover"><Cover {series} eager /></div>
    <CoverActions
      custom={series.cover.custom}
      busy={coverBusy}
      onRefresh={onRefreshCover}
      onUpload={onUploadCover}
    />
  </div>
  <div class="info">
    <h1>{series.title}</h1>
    {#if series.alt_titles.length}
      <p class="muted">{t('series.alt_titles', { titles: series.alt_titles.join(' · ') })}</p>
    {/if}
    <p class="tags">
      <span class="pill">{t(`status.${series.status}`)}</span>
      <span class="pill">{t(`type.${series.type}`)}</span>
      <span class="pill">{t(`category.${series.category}`)}</span>
    </p>
    <p>
      {t('series.progress', {
        read: formatChapter(series.last_read_chapter),
        latest: formatChapter(series.latest_chapter),
        unread: series.unread_count,
      })}
    </p>
    <p>
      {t('series.frequency', { label: frequencyLabel(series.release_interval_days) })}
      {#if release}<br /><span class:overdue>{release}</span>{/if}
    </p>
    <CheckErrorNotice {series} />
    <p class="muted small">
      <a href={series.source.url} target="_blank" rel="noopener noreferrer">
        {t('series.source', { domain: series.source.domain })}
      </a>
      {#if checked}· {t('series.last_checked', { date: checked })}{/if}
    </p>
    {#if series.source.mirrors.length}
      <p class="muted small">
        {t('series.mirrors', {
          domains: series.source.mirrors.map((m) => new URL(m).hostname).join(', '),
        })}
      </p>
    {/if}

    <div class="actions">
      <ContinueButton {series} />
      <button type="button" onclick={onRefresh} disabled={refreshing}>
        {#if refreshing}<Spinner size={14} />{/if}
        {refreshing ? t('series.refreshing') : t('series.refresh')}
      </button>
      <button type="button" onclick={onEdit}>{t('series.edit')}</button>
      <button type="button" class="danger" onclick={onRemove}>{t('series.remove')}</button>
    </div>
  </div>
</div>

<style>
  .header {
    display: grid;
    grid-template-columns: minmax(120px, 220px) 1fr;
    gap: 20px;
    align-items: start;
  }
  @media (max-width: 560px) {
    .header {
      grid-template-columns: 1fr;
    }
    .cover {
      max-width: 200px;
    }
  }
  .cover {
    border-radius: var(--radius);
    overflow: hidden;
    box-shadow: var(--shadow);
  }
  h1 {
    margin: 0 0 4px;
    font-size: 1.5rem;
  }
  p {
    margin: 6px 0;
  }
  .muted {
    color: var(--text-muted);
  }
  .small {
    font-size: 0.85rem;
  }
  .tags {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .pill {
    font-size: 0.8rem;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--surface-2);
  }
  .overdue {
    color: var(--warning);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 12px;
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
    text-decoration: none;
    color: var(--text);
    font-size: 0.95rem;
  }
  .danger {
    color: var(--danger);
  }
</style>
