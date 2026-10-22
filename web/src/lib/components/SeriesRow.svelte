<script lang="ts">
  import Cover from './Cover.svelte';
  import { formatChapter } from '../format';
  import { t } from '../i18n';
  import { href } from '../stores/router.svelte';
  import type { Series } from '../api/types';

  // Compact library row (RF-31): the card's information on one line, with
  // a small cover, for long libraries.
  let { series }: { series: Series } = $props();
</script>

<a class="row" href={href({ name: 'series', id: series.id })}>
  <span class="thumb"><Cover {series} /></span>
  <span class="main">
    <span class="title">{series.title}</span>
    <span class="meta">
      <span class="status status-{series.status}">{t(`status.${series.status}`)}</span>
      <span>{t(`type.${series.type}`)} · {series.source.domain}</span>
    </span>
  </span>
  <span class="progress">
    {t('card.progress', {
      read: formatChapter(series.last_read_chapter),
      latest: formatChapter(series.latest_chapter),
    })}
  </span>
  <span class="badges">
    {#if series.check_error}
      <span class="badge error" title={series.check_error.message}>
        <span aria-hidden="true">!</span>
        <span class="sr-only">{t('card.check_error')}</span>
      </span>
    {/if}
    {#if series.has_new}<span class="badge new">{t('card.new')}</span>{/if}
    {#if series.unread_count > 0}
      <span class="badge unread">{t('card.unread', { n: series.unread_count })}</span>
    {/if}
  </span>
</a>

<style>
  .row {
    display: grid;
    grid-template-columns: 40px minmax(0, 1fr) auto auto;
    align-items: center;
    gap: 12px;
    padding: 6px 10px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: inherit;
    text-decoration: none;
  }
  .row:hover .title {
    color: var(--accent);
  }
  .thumb {
    width: 40px;
    aspect-ratio: 2 / 3;
    overflow: hidden;
    border-radius: 4px;
  }
  .thumb :global(img),
  .thumb :global(.placeholder) {
    width: 100%;
    height: 100%;
    object-fit: cover;
    font-size: 1rem;
  }
  .main {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .title {
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .meta,
  .progress {
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .meta {
    display: flex;
    gap: 8px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .progress {
    white-space: nowrap;
  }
  .badges {
    display: flex;
    gap: 4px;
    justify-content: end;
  }
  .badge {
    font-size: 0.75rem;
    font-weight: 600;
    padding: 2px 6px;
    border-radius: 999px;
    background: var(--bg);
    border: 1px solid var(--border);
    white-space: nowrap;
  }
  .badge.new {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-text);
  }
  .badge.error {
    background: var(--danger);
    border-color: var(--danger);
    color: #fff;
  }
  .status-ongoing {
    color: var(--success);
  }
  .status-hiatus,
  .status-cancelled {
    color: var(--warning);
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }
  @media (max-width: 560px) {
    .row {
      grid-template-columns: 40px minmax(0, 1fr) auto;
    }
    .progress {
      display: none;
    }
  }
</style>
