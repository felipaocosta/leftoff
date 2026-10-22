<script lang="ts">
  import { t } from '../i18n';
  import { CATEGORIES, SERIES_TYPES } from '../api/types';
  import type { Patch, Series } from '../api/types';

  // Edits the user-owned fields of a series (RF-05, RF-07).
  let {
    series,
    onSave,
    onCancel,
  }: {
    series: Series;
    onSave: (patch: Patch, sourceUrl: string) => void;
    onCancel: () => void;
  } = $props();

  // Form fields start from the series; later prop changes are not wanted here.
  // svelte-ignore state_referenced_locally
  let form = $state({
    title: series.title,
    alt: series.alt_titles.join('\n'),
    type: series.type,
    category: series.category,
    tags: series.tags.join(', '),
    notes: series.notes,
    source: series.source.url,
  });

  function submit(e: SubmitEvent) {
    e.preventDefault();
    onSave(
      {
        title: form.title,
        alt_titles: form.alt.split('\n'),
        type: form.type,
        category: form.category,
        tags: form.tags.split(','),
        notes: form.notes,
      },
      form.source.trim(),
    );
  }
</script>

<form onsubmit={submit}>
  <label>{t('edit.title')} <input required bind:value={form.title} /></label>
  <label>{t('edit.alt_titles')} <textarea rows="3" bind:value={form.alt}></textarea></label>
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
  <label>{t('edit.tags')} <input bind:value={form.tags} /></label>
  <label>{t('edit.notes')} <textarea rows="3" bind:value={form.notes}></textarea></label>
  <label>
    {t('edit.source')}
    <input type="url" required bind:value={form.source} />
    <small>{t('edit.source_help')}</small>
  </label>
  <div class="actions">
    <button type="submit" class="primary">{t('edit.save')}</button>
    <button type="button" onclick={onCancel}>{t('edit.cancel')}</button>
  </div>
</form>

<style>
  form {
    display: grid;
    gap: 10px;
    padding: 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }
  label {
    display: grid;
    gap: 4px;
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  input,
  select,
  textarea {
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg);
    color: var(--text);
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
  button {
    padding: 8px 14px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    cursor: pointer;
  }
  .primary {
    background: var(--accent);
    color: var(--accent-text);
    border-color: var(--accent);
  }
</style>
