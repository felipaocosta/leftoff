<script lang="ts">
  import Spinner from './Spinner.svelte';
  import { t } from '../i18n';

  // Cover controls under the cover: re-download it from the site, or
  // upload an image of the user's own.
  let {
    custom,
    busy,
    onRefresh,
    onUpload,
  }: {
    custom: boolean;
    busy: boolean;
    onRefresh: () => void;
    onUpload: (file: File) => void;
  } = $props();

  let input: HTMLInputElement | undefined = $state();

  function picked(e: Event) {
    const el = e.currentTarget as HTMLInputElement;
    const file = el.files?.[0];
    el.value = ''; // allow choosing the same file again
    if (file) onUpload(file);
  }
</script>

<div class="cover-actions">
  {#if custom}<span class="pill">{t('series.cover_custom')}</span>{/if}
  <button type="button" onclick={onRefresh} disabled={busy}>
    {#if busy}<Spinner size={12} />{/if}
    {busy ? t('series.cover_refreshing') : t('series.cover_refresh')}
  </button>
  <button type="button" onclick={() => input?.click()} disabled={busy}>
    {t('series.cover_upload')}
  </button>
  <input
    bind:this={input}
    type="file"
    accept="image/jpeg,image/png,image/gif,image/webp"
    hidden
    onchange={picked}
    aria-label={t('series.cover_upload')}
  />
</div>

<style>
  .cover-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 8px;
  }
  button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    font-size: 0.8rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.6;
  }
  .pill {
    font-size: 0.75rem;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--surface-2);
    align-self: center;
  }
</style>
