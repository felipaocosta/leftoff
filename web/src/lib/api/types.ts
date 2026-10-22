// Mirror of the Go API types (internal/domain, internal/library,
// internal/extract, internal/api). RULES §9: any change to the Go JSON
// updates this file in the same PR.

export type SeriesType = 'manga' | 'manhwa' | 'manhua' | 'novel' | 'other';
export type Status = 'ongoing' | 'completed' | 'hiatus' | 'cancelled' | 'unknown';
export type Category = 'reading' | 'plan_to_read' | 'on_hold' | 'dropped' | 'completed';
export type SortOrder = 'updated' | 'title' | 'unread' | 'added';

export const SERIES_TYPES: readonly SeriesType[] = ['manga', 'manhwa', 'manhua', 'novel', 'other'];
export const STATUSES: readonly Status[] = [
  'ongoing',
  'completed',
  'hiatus',
  'cancelled',
  'unknown',
];
export const CATEGORIES: readonly Category[] = [
  'reading',
  'plan_to_read',
  'on_hold',
  'dropped',
  'completed',
];
export const SORT_ORDERS: readonly SortOrder[] = ['updated', 'title', 'unread', 'added'];

/** "YYYY-MM-DD" (domain.Date). */
export type DateString = string;
/** RFC 3339 timestamp in UTC. */
export type Timestamp = string;

export interface Chapter {
  key: string;
  number: number | null;
  label: string;
  url: string;
  alt_urls: string[] | null;
  released_at: DateString | null;
  discovered_at: Timestamp | null;
  read: boolean;
  read_at: Timestamp | null;
}

export interface Source {
  url: string;
  domain: string;
  method: string;
  mirrors: string[];
}

export interface Cover {
  remote_url: string;
  local: string;
  /** The user uploaded this image; refreshes keep it. */
  custom: boolean;
  /** When the local file last changed (cache-busting version). */
  updated_at: Timestamp | null;
}

/** check_error.code (schema 3): the kind of failure; '' is any other. */
export type CheckErrorCode =
  | ''
  | 'needs_browser'
  | 'no_browser'
  | 'blocked'
  | 'source_changed'
  | 'extraction_incomplete'
  | 'not_found'
  | 'timeout'
  | 'upstream_error';

export interface CheckError {
  code: CheckErrorCode;
  message: string;
  at: Timestamp;
  failures: number;
}

/** A series as returned by the API (domain.Series + computed fields). */
export interface Series {
  id: string;
  type: SeriesType;
  title: string;
  alt_titles: string[];
  source: Source;
  cover: Cover;
  status: Status;
  status_raw: string;
  synopsis: string;
  tags: string[];
  category: Category;
  notes: string;
  latest_chapter: number | null;
  total_chapters: number | null;
  last_read_chapter: number | null;
  added_at: Timestamp;
  last_checked_at: Timestamp | null;
  last_new_chapter_at: Timestamp | null;
  seen_latest_chapter: number | null;
  check_error: CheckError | null;
  external?: External | null;
  release_interval_days: number | null;
  next_release_estimate: DateString | null;
  /** null in list responses, filled in GET /series/{id}. */
  chapters: Chapter[] | null;
  unread_count: number;
  has_new: boolean;
}

export interface SeriesInfo {
  url: string;
  title: string;
  alt_titles: string[] | null;
  cover_url: string;
  status: Status;
  status_raw: string;
  synopsis: string;
  type: SeriesType;
}

export interface ChapterInfo {
  number: number | null;
  label: string;
  url: string;
  alt_urls: string[] | null;
  released_at: DateString | null;
}

export interface PreviewResult {
  series: SeriesInfo;
  chapters: ChapterInfo[];
  method: string;
  confidence: number;
  needs_confirmation: boolean;
}

export interface Overrides {
  title?: string;
  type?: SeriesType;
  cover_url?: string;
  category?: Category;
}

export interface Patch {
  title?: string;
  alt_titles?: string[];
  type?: SeriesType;
  tags?: string[];
  notes?: string;
  category?: Category;
}

export interface SeriesQuery {
  q?: string;
  status?: Status;
  category?: Category;
  type?: SeriesType;
  has_new?: boolean;
  /** The series' current site, e.g. "mangaread.org" (RF-33). */
  source?: string;
  sort?: SortOrder;
}

/** Notification settings (RF-26). Secrets come masked ("••••abcd"); sent
 * back unchanged they keep the stored value. Locked fields come from
 * environment variables and cannot be changed here. */
export interface NotificationSettings {
  ntfy_url: string;
  discord_webhook: string;
  telegram: { token: string; chat_id: string };
  only_reading: boolean;
  language: 'pt-BR' | 'en';
  locked?: string[];
}

/** AniList/MangaUpdates lookups (RF-57); locked by ENRICH_ENABLED. */
export interface EnrichmentSettings {
  enabled: boolean;
  locked?: boolean;
}

/** How the desktop app opens (Phase 6); only in the desktop app. */
export interface DesktopSettings {
  mode: 'standalone' | 'connected';
  server_url: string;
}

export interface SettingsView {
  notifications: NotificationSettings;
  enrichment: EnrichmentSettings;
  desktop?: DesktopSettings;
}

/** PUT /settings: a missing section is left as it is. */
export interface SettingsUpdate {
  notifications?: NotificationSettings;
  enrichment?: EnrichmentSettings;
  desktop?: DesktopSettings;
}

/** The outcome of a test notification on one channel. */
export interface ChannelResult {
  channel: 'ntfy' | 'discord' | 'telegram';
  ok: boolean;
  error?: string;
}

/** A site of the library and how many series are read from it. */
export interface SourceCount {
  domain: string;
  count: number;
}

export interface RefreshResult {
  new_chapters: number;
  series: Series;
}

export interface Health {
  status: string;
  version: string;
  /** "app" for the desktop app (offers "quit"), "server" otherwise. */
  mode: 'app' | 'server' | 'desktop' | 'android';
  /** A language model is configured and can generate recipes (RF-54). */
  llm: boolean;
  /** Anti-bot challenge solvers in use (RF-56): "flaresolverr", "browser". */
  solvers: string[];
}

/** A stored recipe (GET /recipes). */
export interface RecipeInfo {
  domain: string;
  aliases: string[];
  generated_by: string;
  updated_at: Timestamp;
  /** Set when the stored file is invalid. */
  error?: string;
  /** Series this recipe no longer reads (RF-55). */
  failing: FailingSeries[];
}

/** A series whose site recipe stopped working: it fell back to the
 * heuristic or failed several checks in a row. */
/** The series' entry in AniList or MangaUpdates (RF-57). null: never
 * looked up; empty id: not found, or a wrong match the user removed. */
export interface External {
  source: '' | 'anilist' | 'mangaupdates';
  id: string;
  url: string;
  title: string;
  status: Status;
  genres: string[] | null;
  year: number | null;
  synopsis: string;
  ignored: boolean;
  checked_at: Timestamp;
}

export interface FailingSeries {
  series_id: string;
  title: string;
  url: string;
  reason: 'fallback' | 'failing';
  message?: string;
}

/** Outcome of POST /recipes/{domain}/test. */
export interface RecipeTest {
  result: PreviewResult;
  problems: string[];
}

/** A recipe written by the language model: tested on the page, not saved. */
export interface GeneratedRecipe extends RecipeTest {
  /** The recipe's JSON text, for the editor. */
  recipe: string;
  /** What a working recipe could still do better (e.g. no dates). */
  notes: string[];
  attempts: number;
}

/** A background "refresh all" job (POST /refresh, GET /jobs/{id}). */
export interface Job {
  id: string;
  status: 'running' | 'done';
  total: number;
  done: number;
  failed: number;
  new_chapters: number;
  started_at: Timestamp;
  finished_at: Timestamp | null;
}

/** Summary of POST /import (RF-43). */
export interface ImportReport {
  added: number;
  merged: number;
  chapters_read: number;
}

export interface ErrorBody {
  error: { code: string; message: string };
}
