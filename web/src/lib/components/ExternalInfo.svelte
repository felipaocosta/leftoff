<script lang="ts">
  import Spinner from './Spinner.svelte';
  import { api } from '../api';
  import { errorMessage } from '../errors';
  import { formatDateTime } from '../format';
  import { t } from '../i18n';
  import type { Series } from '../api/types';

  // The series' AniList/MangaUpdates entry (RF-57): official status, year,
  // genres and link, with actions to look it up, fix a wrong match and turn
  // the genres into tags.
  let { series, onChange }: { series: Series; onChange: (s: Series) => void } = $props();

  let busy = $state(false);
  let error = $state('');

  let ext = $derived(series.external ?? null);
  let found = $derived(ext !== null && ext.id !== '' && !ext.ignored);
  let source = $derived(ext?.source ? t(`external.source.${ext.source}`) : '');
  let newGenres = $derived(
    (ext?.genres ?? []).filter(
      (g) => !series.tags.some((tag) => tag.toLowerCase() === g.toLowerCase()),
    ),
  );

  async function run(call: () => Promise<Series>) {
    busy = true;
    error = '';
    try {
      onChange(await call());
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const lookup = () => run(() => api.enrichSeries(series.id));
  const addTags = () =>
    run(() => api.editSeries(series.id, { tags: [...series.tags, ...newGenres] }));
  function unlink() {
    if (window.confirm(t('external.unlink_confirm', { source })))
      run(() => api.unlinkSeries(series.id));
  }
</script>

<section class="external">
  {#if found && ext}
    <p>
      {t('external.official', { source, status: t(`status.${ext.status}`) })}
      {#if ext.year}· {t('external.year', { year: ext.year })}{/if}
      ·
      <a href={ext.url} target="_blank" rel="noopener noreferrer"
        >{t('external.open', { source })}</a
      >
    </p>
    {#if ext.genres?.length}<p class="muted">
        {t('external.genres', { genres: ext.genres.join(', ') })}
      </p>{/if}
  {:else if ext?.ignored}
    <p class="muted">{t('external.ignored')}</p>
  {:else if ext}
    <p class="muted">{t('external.not_found', { date: formatDateTime(ext.checked_at) })}</p>
  {/if}
  <div class="actions">
    {#if found}
      {#if newGenres.length > 0}
        <button type="button" onclick={addTags} disabled={busy}>{t('external.add_tags')}</button>
      {/if}
      <button type="button" onclick={unlink} disabled={busy}>{t('external.unlink')}</button>
    {:else}
      <button type="button" onclick={lookup} disabled={busy}>
        {#if busy}<Spinner size={14} />{/if}
        {busy ? t('external.looking') : ext ? t('external.lookup_again') : t('external.lookup')}
      </button>
    {/if}
  </div>
  {#if error}<p class="error" role="alert">{error}</p>{/if}
</section>

<style>
  .external {
    margin: 12px 0;
    font-size: 0.9rem;
  }
  p {
    margin: 0 0 4px;
  }
  .muted {
    color: var(--text-muted);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 6px;
  }
  button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    font-size: 0.8rem;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    cursor: pointer;
  }
  .error {
    color: var(--danger);
  }
</style>
