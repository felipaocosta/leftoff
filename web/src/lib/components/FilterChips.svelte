<script lang="ts">
  import { t } from '../i18n';
  import type { SeriesQuery } from '../api/types';

  // Quick filters as chips (one tap) and the active filters with a remove
  // button, visible even when the filter panel is closed.
  let { query = $bindable() }: { query: SeriesQuery } = $props();

  type Field = 'has_new' | 'category' | 'status' | 'type' | 'source';
  interface Chip {
    id: string;
    label: string;
    on: boolean;
    toggle: () => void;
  }

  function clear(field: Field) {
    query = { ...query, [field]: undefined };
  }

  let chips = $derived.by((): Chip[] => {
    const out: Chip[] = [
      {
        id: 'new',
        label: t('library.filter.new'),
        on: query.has_new === true,
        toggle: () => (query = { ...query, has_new: query.has_new === true ? undefined : true }),
      },
      {
        id: 'reading',
        label: t('category.reading'),
        on: query.category === 'reading',
        toggle: () =>
          (query = { ...query, category: query.category === 'reading' ? undefined : 'reading' }),
      },
    ];
    // Other active filters, removable.
    if (query.has_new === false) out.push(active('up', t('library.filter.up_to_date'), 'has_new'));
    if (query.category && query.category !== 'reading')
      out.push(active('category', t(`category.${query.category}`), 'category'));
    if (query.status) out.push(active('status', t(`status.${query.status}`), 'status'));
    if (query.type) out.push(active('type', t(`type.${query.type}`), 'type'));
    if (query.source) out.push(active('source', query.source, 'source'));
    return out;
  });

  function active(id: string, label: string, field: Field): Chip {
    return { id, label, on: true, toggle: () => clear(field) };
  }
</script>

<div class="chips" role="group" aria-label={t('library.filters')}>
  {#each chips as c (c.id)}
    <button type="button" class="chip" aria-pressed={c.on} onclick={c.toggle}>
      {c.label}
      {#if c.on}<span aria-hidden="true">×</span>{/if}
    </button>
  {/each}
</div>

<style>
  .chips {
    display: flex;
    gap: 6px;
    overflow-x: auto;
    padding-bottom: 4px;
    margin-bottom: 8px;
    scrollbar-width: none;
  }
  .chip {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--surface);
    color: var(--text);
    font-size: 0.85rem;
    cursor: pointer;
    white-space: nowrap;
  }
  .chip[aria-pressed='true'] {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-text);
  }
</style>
