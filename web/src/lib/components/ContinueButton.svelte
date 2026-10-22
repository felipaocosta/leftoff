<script lang="ts">
  import { t } from '../i18n';
  import { nextUnread } from '../series';
  import type { Series } from '../api/types';

  // "Continue" (RF-13): opens the next unread chapter on the site. On
  // phones it floats above the tab bar, one thumb away while scrolling the
  // chapter list; the page keeps room for it (body.has-sticky-action).
  let { series }: { series: Series } = $props();

  let next = $derived(nextUnread(series));

  $effect(() => {
    document.body.classList.add('has-sticky-action');
    return () => document.body.classList.remove('has-sticky-action');
  });
</script>

{#if next}
  <a
    class="continue"
    href={next.url || series.source.url}
    target="_blank"
    rel="noopener noreferrer"
  >
    {t('series.continue', { label: next.label })}
  </a>
{:else}
  <span class="continue done">{t('series.all_read')}</span>
{/if}

<style>
  .continue {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 8px 14px;
    border: 1px solid var(--accent);
    border-radius: 8px;
    background: var(--accent);
    color: var(--accent-text);
    text-decoration: none;
    font-weight: 600;
  }
  .done {
    background: var(--surface-2);
    border-color: var(--border);
    color: var(--text-muted);
    font-weight: 400;
  }
  @media (max-width: 640px) {
    .continue {
      position: fixed;
      left: 12px;
      right: 12px;
      bottom: calc(70px + env(safe-area-inset-bottom));
      z-index: 9;
      padding: 12px 16px;
      border-radius: 12px;
      box-shadow: 0 4px 16px rgb(0 0 0 / 0.35);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .done {
      position: static;
      box-shadow: none;
    }
  }
</style>
