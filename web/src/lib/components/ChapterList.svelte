<script lang="ts">
  import { formatChapter, formatDate } from '../format';
  import { swipe } from '../actions/swipe';
  import { t } from '../i18n';
  import type { Chapter } from '../api/types';

  // Chapter list with read checkboxes and "mark up to here" (RF-10, RF-11,
  // RF-35). Chapters open on the source site (the app is not a reader).
  // Chapters without their own link (e.g. novelupdates releases) open the
  // series page (fallbackUrl). On touch screens a sideways swipe toggles
  // a chapter and a long press marks every chapter up to it as read.
  let {
    chapters,
    fallbackUrl,
    onToggle,
    onUpTo,
  }: {
    chapters: Chapter[];
    fallbackUrl: string;
    onToggle: (chapter: Chapter, read: boolean) => void;
    onUpTo: (chapter: Chapter) => void;
  } = $props();
</script>

{#if chapters.length === 0}
  <p class="empty">{t('series.no_chapters')}</p>
{:else}
  <ul>
    {#each chapters as c (c.key)}
      <li
        class:read={c.read}
        use:swipe={{
          onSwipe: () => onToggle(c, !c.read),
          onLongPress: c.number !== null ? () => onUpTo(c) : undefined,
        }}
      >
        <label class="check">
          <input
            type="checkbox"
            checked={c.read}
            aria-label={t('series.read_toggle', { label: c.label })}
            onchange={(e) => onToggle(c, e.currentTarget.checked)}
          />
        </label>
        <a href={c.url || fallbackUrl} target="_blank" rel="noopener noreferrer">{c.label}</a>
        <span class="date">{formatDate(c.released_at)}</span>
        {#if c.number !== null}
          <button
            type="button"
            title={t('series.mark_up_to')}
            aria-label={t('series.mark_up_to_label', { label: formatChapter(c.number) })}
            onclick={() => onUpTo(c)}
          >
            ⤓
          </button>
        {/if}
      </li>
    {/each}
  </ul>
{/if}

<style>
  ul {
    list-style: none;
    padding: 0;
    margin: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }
  li {
    display: grid;
    grid-template-columns: auto 1fr auto 2.4rem;
    align-items: center;
    gap: 8px;
    min-height: 48px;
    padding: 0 12px 0 4px;
    border-bottom: 1px solid var(--border);
    transform: translateX(var(--swipe-x, 0));
    transition: transform 0.15s;
    user-select: none;
  }
  li:global(.swiping) {
    transition: none;
    background: var(--surface-2);
  }
  /* The whole left edge toggles the chapter (a large touch target). */
  .check {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    height: 44px;
    cursor: pointer;
  }
  li:last-child {
    border-bottom: 0;
  }
  li.read a {
    color: var(--text-muted);
  }
  input {
    width: 1.3rem;
    height: 1.3rem;
  }
  a {
    text-decoration: none;
    overflow-wrap: anywhere;
  }
  .date {
    font-size: 0.8rem;
    color: var(--text-muted);
    white-space: nowrap;
  }
  button {
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface-2);
    cursor: pointer;
    padding: 6px 0;
  }
  .empty {
    color: var(--text-muted);
  }
</style>
