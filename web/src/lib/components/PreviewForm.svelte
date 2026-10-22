<script lang="ts">
  import Spinner from './Spinner.svelte';
  import { formatChapter } from '../format';
  import { t } from '../i18n';
  import { CATEGORIES, SERIES_TYPES } from '../api/types';
  import type { Category, Overrides, PreviewResult, SeriesType } from '../api/types';

  // Editable preview of an extraction (RF-02). Only changed fields are sent
  // as overrides; low-confidence results need an explicit confirmation.
  let {
    preview,
    saving,
    onSave,
  }: { preview: PreviewResult; saving: boolean; onSave: (o: Overrides) => void } = $props();

  // The form starts from the preview; later prop changes are not wanted here.
  // svelte-ignore state_referenced_locally
  let form = $state({
    title: preview.series.title,
    type: preview.series.type as SeriesType,
    category: 'reading' as Category,
    cover: preview.series.cover_url,
  });
  let confirmed = $state(false);

  let latest = $derived(Math.max(-Infinity, ...preview.chapters.map((c) => c.number ?? -Infinity)));
  let canSave = $derived(
    !saving && form.title.trim() !== '' && (!preview.needs_confirmation || confirmed),
  );

  function submit(e: SubmitEvent) {
    e.preventDefault();
    const o: Overrides = { category: form.category };
    if (form.title.trim() !== preview.series.title) o.title = form.title.trim();
    if (form.type !== preview.series.type) o.type = form.type;
    if (form.cover.trim() !== preview.series.cover_url) o.cover_url = form.cover.trim();
    onSave(o);
  }
</script>

<form onsubmit={submit}>
  <h2>{t('add.preview')}</h2>
  <div class="layout">
    <div class="cover">
      {#if form.cover}<img src={form.cover} alt={form.title} referrerpolicy="no-referrer" />{/if}
    </div>
    <div class="fields">
      <label>{t('edit.title')} <input required bind:value={form.title} /></label>
      <div class="row">
        <label>
          {t('edit.type')}
          <select bind:value={form.type}>
            {#each SERIES_TYPES as ty (ty)}<option value={ty}>{t(`type.${ty}`)}</option>{/each}
          </select>
        </label>
        <label>
          {t('edit.category')}
          <select bind:value={form.category}>
            {#each CATEGORIES as c (c)}<option value={c}>{t(`category.${c}`)}</option>{/each}
          </select>
        </label>
      </div>
      <label>{t('add.cover_url')} <input type="url" bind:value={form.cover} /></label>
      <p>{t('add.status', { status: t(`status.${preview.series.status}`) })}</p>
      <p>
        {t('add.chapters', {
          n: preview.chapters.length,
          latest: formatChapter(Number.isFinite(latest) ? latest : null),
        })}
      </p>
      <p class="muted">
        {t('add.method', { method: preview.method, percent: Math.round(preview.confidence * 100) })}
      </p>
      {#if preview.needs_confirmation}
        <p class="warning" role="status">{t('add.low_confidence')}</p>
        <label class="check"
          ><input type="checkbox" bind:checked={confirmed} /> {t('add.confirm')}</label
        >
      {/if}
      <button type="submit" disabled={!canSave}>
        {#if saving}<Spinner size={14} />{/if}
        {saving ? t('add.saving') : t('add.save')}
      </button>
    </div>
  </div>
</form>

<style>
  form {
    margin-top: 16px;
    padding: 16px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }
  h2 {
    margin: 0 0 12px;
    font-size: 1.1rem;
  }
  .layout {
    display: grid;
    grid-template-columns: minmax(100px, 180px) 1fr;
    gap: 16px;
  }
  @media (max-width: 560px) {
    .layout {
      grid-template-columns: 1fr;
    }
  }
  img {
    width: 100%;
    aspect-ratio: 2 / 3;
    object-fit: cover;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .fields {
    display: grid;
    gap: 10px;
    align-content: start;
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }
  label {
    display: grid;
    gap: 4px;
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  .check {
    display: flex;
    gap: 8px;
    align-items: center;
    color: var(--text);
  }
  input:not([type='checkbox']),
  select {
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg);
    color: var(--text);
  }
  p {
    margin: 0;
  }
  .muted {
    color: var(--text-muted);
    font-size: 0.85rem;
  }
  .warning {
    color: var(--warning);
  }
  button {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    justify-self: start;
    padding: 10px 16px;
    border: 0;
    border-radius: 8px;
    background: var(--accent);
    color: var(--accent-text);
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
