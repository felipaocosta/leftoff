<script lang="ts">
  import { formatChapter, formatDate } from '../format';
  import { t } from '../i18n';
  import type { RecipeTest } from '../api/types';

  // What a recipe extracted from the test page, what it missed and, for a
  // generated recipe, what it could do better (notes).
  let { test, notes = [] }: { test: RecipeTest; notes?: string[] } = $props();
  let s = $derived(test.result.series);
</script>

<div class="result" class:bad={test.problems.length > 0}>
  {#if test.problems.length === 0}
    <p class="ok" role="status">{t('recipes.ok')}</p>
    {#if notes.length > 0}
      <p class="notes">{t('recipes.notes')}</p>
      <ul class="notes">
        {#each notes as n (n)}<li>{n}</li>{/each}
      </ul>
    {/if}
  {:else}
    <p role="alert">{t('recipes.problems')}</p>
    <ul>
      {#each test.problems as p (p)}<li>{p}</li>{/each}
    </ul>
  {/if}
  <div class="series">
    {#if s.cover_url}<img src={s.cover_url} alt="" referrerpolicy="no-referrer" />{/if}
    <div>
      <strong>{s.title || '—'}</strong>
      <p>
        {[
          ...(s.alt_titles ?? []),
          s.status_raw || (s.status === 'unknown' ? '' : s.status),
          t('recipes.chapter_count', { n: test.result.chapters.length }),
        ]
          .filter(Boolean)
          .join(' · ')}
      </p>
      <ol>
        {#each test.result.chapters.slice(0, 5) as c (c.url + c.label)}
          <li>
            {formatChapter(c.number)} — {c.label}
            {#if c.released_at}<small>({formatDate(c.released_at)})</small>{/if}
          </li>
        {/each}
      </ol>
    </div>
  </div>
</div>

<style>
  .notes {
    margin: 0 0 6px;
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  .result {
    border: 1px solid var(--success);
    border-radius: var(--radius);
    padding: 10px 12px;
    background: var(--surface);
  }
  .result.bad {
    border-color: var(--warning);
  }
  .ok {
    color: var(--success);
  }
  .series {
    display: flex;
    gap: 12px;
  }
  img {
    width: 72px;
    aspect-ratio: 2 / 3;
    object-fit: cover;
    border-radius: 6px;
  }
  ol,
  ul {
    margin: 4px 0;
    padding-left: 18px;
    font-size: 0.85rem;
  }
  p {
    margin: 4px 0;
  }
</style>
