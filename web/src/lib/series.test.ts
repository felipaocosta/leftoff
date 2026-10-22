import { describe, expect, it } from 'vitest';
import { nextUnread, withRead, withReadUpTo } from './series';
import { makeSeries } from './test-fixtures';
import type { Chapter } from './api/types';

function ch(key: string, number: number | null, read = false): Chapter {
  return {
    key,
    number,
    label: number === null ? key : `Chapter ${number}`,
    url: `https://site.org/${key}`,
    alt_urls: [],
    released_at: null,
    discovered_at: null,
    read,
    read_at: read ? '2026-10-01T00:00:00Z' : null,
  };
}

// Newest first, like the sources.
const chapters = [ch('4', 4), ch('3', 3), ch('extra', null), ch('2', 2, true), ch('1', 1, true)];

describe('nextUnread', () => {
  it('picks the first unread number after the last read one', () => {
    expect(nextUnread(makeSeries({ chapters, last_read_chapter: 2 }))?.key).toBe('3');
  });
  it('falls back to the lowest unread when the read ones are ahead', () => {
    const s = makeSeries({ chapters: [ch('2', 2, true), ch('1', 1)], last_read_chapter: 2 });
    expect(nextUnread(s)?.key).toBe('1');
  });
  it('uses source order for chapters without numbers', () => {
    const s = makeSeries({ chapters: [ch('b', null), ch('a', null)], last_read_chapter: null });
    expect(nextUnread(s)?.key).toBe('a');
  });
  it('returns undefined when everything is read', () => {
    expect(nextUnread(makeSeries({ chapters: [ch('1', 1, true)] }))).toBeUndefined();
  });
});

describe('optimistic read updates', () => {
  const now = new Date('2026-10-19T12:00:00Z');
  const s = makeSeries({ chapters, last_read_chapter: 2, unread_count: 3 });

  it('marks and unmarks chapters, recomputing counters', () => {
    const read = withRead(s, ['4'], true, now);
    expect(read.last_read_chapter).toBe(4);
    expect(read.unread_count).toBe(2);
    expect(read.chapters?.[0]?.read_at).toBe('2026-10-19T12:00:00.000Z');

    const unread = withRead(read, ['4', '2'], false, now);
    expect(unread.last_read_chapter).toBe(1);
    expect(unread.chapters?.[0]?.read_at).toBeNull();
  });

  it('keeps the original read_at of chapters already read', () => {
    expect(withRead(s, ['2'], true, now).chapters?.[3]?.read_at).toBe('2026-10-01T00:00:00Z');
  });

  it('marks numbered chapters up to a number, leaving extras alone', () => {
    const up = withReadUpTo(s, 3, now);
    expect(up.chapters?.map((c) => c.read)).toEqual([false, true, false, true, true]);
    expect(up.last_read_chapter).toBe(3);
    expect(s.chapters?.[1]?.read).toBe(false); // the original is not mutated
  });
});
