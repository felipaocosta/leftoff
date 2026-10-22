import type { Series } from './api/types';

/** A complete Series with sensible defaults for component tests. */
export function makeSeries(overrides: Partial<Series> = {}): Series {
  return {
    id: '01TEST',
    type: 'manhua',
    title: 'Solo Leveling',
    alt_titles: [],
    source: {
      url: 'https://site.org/manga/solo',
      domain: 'site.org',
      method: 'adapter:test',
      mirrors: [],
    },
    cover: {
      remote_url: 'https://site.org/c.jpg',
      local: 'covers/01TEST.jpg',
      custom: false,
      updated_at: null,
    },
    status: 'ongoing',
    status_raw: 'OnGoing',
    synopsis: 'A hunter.',
    tags: [],
    category: 'reading',
    notes: '',
    latest_chapter: 120,
    total_chapters: null,
    last_read_chapter: 100,
    added_at: '2026-10-01T10:00:00Z',
    last_checked_at: '2026-10-19T12:00:00Z',
    last_new_chapter_at: null,
    seen_latest_chapter: 120,
    check_error: null,
    release_interval_days: 7,
    next_release_estimate: '2026-10-22',
    chapters: null,
    unread_count: 20,
    has_new: false,
    ...overrides,
  };
}
