<script lang="ts">
  import { t } from '../i18n';
  import { CATEGORIES, SERIES_TYPES, SORT_ORDERS, STATUSES } from '../api/types';
  import type { SeriesQuery, SourceCount } from '../api/types';
  import FilterChips from './FilterChips.svelte';
  import { prefs } from '../stores/prefs.svelte';

  // Search, filters and sort for the library (RF-32, RF-33, RF-34). The
  // source filter only shows when the library uses more than one site.
  let { query = $bindable(), sources = [] }: { query: SeriesQuery; sources?: SourceCount[] } =
    $props();

  // How many filters are set, shown on the collapsed "Filters" button.
  let active = $derived(
    [query.status, query.category, query.type, query.source, query.has_new].filter(
      (v) => v !== undefined && v !== '',
    ).length,
  );

  let newsValue = $derived(query.has_new === undefined ? '' : String(query.has_new));
  function setNews(v: string) {
    query = { ...query, has_new: v === '' ? undefined : v === 'true' };
  }
</script>

<div class="search">
  <input
    type="search"
    placeholder={t('library.search')}
    aria-label={t('library.search')}
    bind:value={query.q}
  />
  <button
    type="button"
    class="toggle"
    aria-expanded={prefs.filtersOpen}
    aria-controls="library-filters"
    onclick={() => prefs.setFiltersOpen(!prefs.filtersOpen)}
  >
    {t('library.filters')}
    {#if active > 0}<span class="count">{active}</span>{/if}
    <svg class="chevron" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
  </button>
</div>
<FilterChips bind:query />
<div class="toolbar" id="library-filters" hidden={!prefs.filtersOpen}>
  <label>
    <span>{t('library.filter.news')}</span>
    <select value={newsValue} onchange={(e) => setNews(e.currentTarget.value)}>
      <option value="">{t('library.filter.all')}</option>
      <option value="true">{t('library.filter.new')}</option>
      <option value="false">{t('library.filter.up_to_date')}</option>
    </select>
  </label>
  <label>
    <span>{t('library.filter.status')}</span>
    <select bind:value={query.status}>
      <option value={undefined}>{t('library.filter.any')}</option>
      {#each STATUSES as s (s)}<option value={s}>{t(`status.${s}`)}</option>{/each}
    </select>
  </label>
  <label>
    <span>{t('library.filter.category')}</span>
    <select bind:value={query.category}>
      <option value={undefined}>{t('library.filter.any')}</option>
      {#each CATEGORIES as c (c)}<option value={c}>{t(`category.${c}`)}</option>{/each}
    </select>
  </label>
  <label>
    <span>{t('library.filter.type')}</span>
    <select bind:value={query.type}>
      <option value={undefined}>{t('library.filter.any')}</option>
      {#each SERIES_TYPES as ty (ty)}<option value={ty}>{t(`type.${ty}`)}</option>{/each}
    </select>
  </label>
  {#if sources.length > 1}
    <label>
      <span>{t('library.filter.source')}</span>
      <select bind:value={query.source}>
        <option value={undefined}>{t('library.filter.any')}</option>
        {#each sources as src (src.domain)}
          <option value={src.domain}>
            {t('library.filter.source_option', { domain: src.domain, n: src.count })}
          </option>
        {/each}
      </select>
    </label>
  {/if}
  <label>
    <span>{t('library.sort')}</span>
    <select bind:value={query.sort}>
      {#each SORT_ORDERS as o (o)}<option value={o}>{t(`library.sort.${o}`)}</option>{/each}
    </select>
  </label>
</div>

<style>
  .search {
    display: flex;
    gap: 8px;
    margin-bottom: 10px;
  }
  .search input {
    flex: 1;
    min-width: 0;
  }
  .toggle {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    white-space: nowrap;
  }
  .count {
    padding: 0 6px;
    border-radius: 999px;
    background: var(--accent);
    color: var(--accent-text);
    font-size: 0.75rem;
    font-weight: 700;
  }
  .chevron {
    width: 16px;
    height: 16px;
    fill: none;
    stroke: currentColor;
    stroke-width: 2;
    transition: transform 0.15s;
  }
  .toggle[aria-expanded='true'] .chevron {
    transform: rotate(180deg);
  }
  .toolbar[hidden] {
    display: none;
  }
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 12px;
    align-items: end;
    margin-bottom: 16px;
  }
  input,
  select {
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
  }
  label {
    display: flex;
    flex-direction: column;
    font-size: 0.75rem;
    color: var(--text-muted);
    gap: 2px;
  }
</style>
