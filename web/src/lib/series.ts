import type { Chapter, Series } from './api/types';

/**
 * The chapter "Continue" opens (RF-13): the lowest-numbered unread chapter
 * after the last read one (or the lowest unread if nothing was read). When
 * no unread chapter has a number, the oldest unread one in source order
 * (sources list newest first).
 */
export function nextUnread(series: Series): Chapter | undefined {
  const chapters = series.chapters ?? [];
  const after = series.last_read_chapter ?? -Infinity;
  const numbered = chapters
    .filter((c) => !c.read && c.number !== null)
    .sort((a, b) => (a.number ?? 0) - (b.number ?? 0));
  return (
    numbered.find((c) => (c.number ?? 0) > after) ??
    numbered[0] ??
    [...chapters].reverse().find((c) => !c.read)
  );
}

/** Optimistic copy of series with the given chapters marked read/unread. */
export function withRead(series: Series, keys: string[], read: boolean, now = new Date()): Series {
  const set = new Set(keys);
  return recompute(series, (c) => (set.has(c.key) ? setRead(c, read, now) : c));
}

/** Optimistic copy of series with every numbered chapter ≤ upTo read. */
export function withReadUpTo(series: Series, upTo: number, now = new Date()): Series {
  return recompute(series, (c) =>
    c.number !== null && c.number <= upTo ? setRead(c, true, now) : c,
  );
}

function setRead(c: Chapter, read: boolean, now: Date): Chapter {
  if (read === c.read) return c;
  return { ...c, read, read_at: read ? now.toISOString() : null };
}

function recompute(series: Series, fn: (c: Chapter) => Chapter): Series {
  const chapters = (series.chapters ?? []).map(fn);
  const readNumbers = chapters
    .filter((c) => c.read && c.number !== null)
    .map((c) => c.number as number);
  return {
    ...series,
    chapters,
    unread_count: chapters.filter((c) => !c.read).length,
    last_read_chapter: readNumbers.length ? Math.max(...readNumbers) : null,
  };
}
