<script lang="ts">
  import { api } from '../api';
  import { t } from '../i18n';
  import type { Series } from '../api/types';

  // Shows the local cover, falling back to the remote URL and then to a
  // placeholder when an image fails to load (RF-41).
  let { series, eager = false }: { series: Series; eager?: boolean } = $props();

  // The file keeps its URL when replaced; its update time makes the URL
  // change, so every screen shows the new cover (no stale browser cache).
  let version = $derived(series.cover.updated_at ? Date.parse(series.cover.updated_at) : 0);

  let sources = $derived(
    [
      series.cover.local ? api.coverUrl(series.id, version) : '',
      series.cover.custom ? '' : series.cover.remote_url,
    ].filter(Boolean),
  );
  let attempt = $state(0);
  $effect(() => {
    void version; // a new cover version starts from the local image again
    attempt = 0;
  });
  let src = $derived(sources[attempt]);
</script>

{#if src}
  <img
    {src}
    alt={series.title}
    loading={eager ? 'eager' : 'lazy'}
    referrerpolicy="no-referrer"
    onerror={() => attempt++}
  />
{:else}
  <div class="placeholder" role="img" aria-label={t('card.no_cover')}>
    <span aria-hidden="true">{series.title.slice(0, 1)}</span>
  </div>
{/if}

<style>
  img,
  .placeholder {
    width: 100%;
    aspect-ratio: 2 / 3;
    object-fit: cover;
    display: block;
    background: var(--surface-2);
  }
  .placeholder {
    display: grid;
    place-items: center;
    font-size: 2.5rem;
    color: var(--text-muted);
  }
</style>
