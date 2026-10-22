<script lang="ts">
  import ExternalInfo from './ExternalInfo.svelte';
  import { t } from '../i18n';
  import type { Series } from '../api/types';

  // Synopsis (the site's, or AniList/MangaUpdates' when the site has
  // none) and the series' external entry (RF-35, RF-57).
  let { series, onChange }: { series: Series; onChange: (s: Series) => void } = $props();

  let external = $derived(series.external?.id && !series.external.ignored ? series.external : null);
  let synopsis = $derived(series.synopsis || external?.synopsis || '');
</script>

{#if synopsis}
  <h2>{t('series.synopsis')}</h2>
  <p class="synopsis">{synopsis}</p>
  {#if !series.synopsis && external}
    <p class="from">
      {t('external.synopsis_from', {
        source: t(`external.source.${external.source || 'anilist'}`),
      })}
    </p>
  {/if}
{/if}
<ExternalInfo {series} {onChange} />

<style>
  h2 {
    font-size: 1.1rem;
    margin: 24px 0 8px;
  }
  .synopsis {
    white-space: pre-line;
    max-width: 75ch;
  }
  .from {
    margin-top: -6px;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
</style>
