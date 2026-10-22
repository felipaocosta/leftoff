<script lang="ts">
  import Cover from './Cover.svelte';
  import { formatChapter } from '../format';
  import { t } from '../i18n';
  import { href } from '../stores/router.svelte';
  import type { Series } from '../api/types';

  // Library grid card (RF-30): cover, title, status, read/latest and badges.
  let { series }: { series: Series } = $props();
</script>

<a class="card" href={href({ name: 'series', id: series.id })}>
  <div class="cover">
    <Cover {series} />
    <div class="badges">
      {#if series.has_new}<span class="badge new">{t('card.new')}</span>{/if}
      {#if series.unread_count > 0}
        <span class="badge unread">{t('card.unread', { n: series.unread_count })}</span>
      {/if}
    </div>
    {#if series.check_error}
      <span class="badge error" title={series.check_error.message}>
        <span aria-hidden="true">!</span>
        <span class="sr-only">{t('card.check_error')}</span>
      </span>
    {/if}
  </div>
  <div class="body">
    <h2 class="title">{series.title}</h2>
    <p class="meta">
      <span class="status status-{series.status}">{t(`status.${series.status}`)}</span>
      <span>
        {t('card.progress', {
          read: formatChapter(series.last_read_chapter),
          latest: formatChapter(series.latest_chapter),
        })}
      </span>
    </p>
  </div>
</a>

<style>
  .card {
    display: flex;
    flex-direction: column;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
    color: inherit;
    text-decoration: none;
    box-shadow: var(--shadow);
  }
  .card:hover .title {
    color: var(--accent);
  }
  .cover {
    position: relative;
  }
  .badges {
    position: absolute;
    top: 6px;
    left: 6px;
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  .badge {
    font-size: 0.75rem;
    font-weight: 600;
    padding: 2px 6px;
    border-radius: 999px;
    background: var(--surface);
    color: var(--text);
  }
  .badge.new {
    background: var(--accent);
    color: var(--accent-text);
  }
  .badge.error {
    position: absolute;
    top: 6px;
    right: 6px;
    background: var(--danger);
    color: #fff;
  }
  .body {
    padding: 8px 10px 10px;
  }
  .title {
    font-size: 0.95rem;
    margin: 0 0 4px;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .meta {
    margin: 0;
    font-size: 0.8rem;
    color: var(--text-muted);
    display: flex;
    flex-wrap: wrap;
    gap: 2px 8px;
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
</style>
