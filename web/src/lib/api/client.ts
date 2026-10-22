import type {
  ChannelResult,
  ErrorBody,
  GeneratedRecipe,
  Health,
  ImportReport,
  Job,
  Overrides,
  Patch,
  PreviewResult,
  RecipeInfo,
  RecipeTest,
  RefreshResult,
  Series,
  SeriesQuery,
  SettingsUpdate,
  SettingsView,
  SourceCount,
} from './types';

const BASE = 'api/v1';

/** An error response from the API ({ error: { code, message } }). */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

type Fetch = typeof fetch;

/** Typed client for /api/v1 (SPEC §7). */
export class ApiClient {
  /** Called after every request with whether the server was reachable. */
  onReachable?: (reachable: boolean) => void;

  constructor(
    private readonly token: () => string = () => '',
    private readonly fetchFn: Fetch = (...args) => fetch(...args),
  ) {}

  health(): Promise<Health> {
    return this.request('GET', '/health');
  }

  preview(url: string): Promise<PreviewResult> {
    return this.request('POST', '/extract/preview', { url });
  }

  async listSeries(query: SeriesQuery = {}): Promise<Series[]> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== '') params.set(key, String(value));
    }
    const qs = params.toString();
    const body = await this.request<{ series: Series[] }>('GET', '/series' + (qs ? '?' + qs : ''));
    return body.series;
  }

  /** Desktop app: saves the export with a native "save as" dialog. */
  saveExportDesktop(): Promise<{ canceled: boolean; path?: string }> {
    return this.request('POST', '/app/export');
  }

  /** Looks the series up in AniList/MangaUpdates now (RF-57). */
  enrichSeries(id: string): Promise<Series> {
    return this.request('POST', `/series/${encodeURIComponent(id)}/enrich`);
  }

  /** Removes a wrong AniList/MangaUpdates match. */
  unlinkSeries(id: string): Promise<Series> {
    return this.request('DELETE', `/series/${encodeURIComponent(id)}/external`);
  }

  getSettings(): Promise<SettingsView> {
    return this.request('GET', '/settings');
  }

  saveSettings(settings: SettingsUpdate): Promise<SettingsView> {
    return this.request('PUT', '/settings', settings);
  }

  /** Sends a test notification with the saved settings (RF-26). */
  async testNotifications(): Promise<ChannelResult[]> {
    return (
      await this.request<{ results: ChannelResult[] }>('POST', '/settings/notifications/test')
    ).results;
  }

  /** The library's sites, for the source filter (RF-33). */
  async listSources(): Promise<SourceCount[]> {
    return (await this.request<{ sources: SourceCount[] }>('GET', '/sources')).sources;
  }

  getSeries(id: string): Promise<Series> {
    return this.request('GET', `/series/${encodeURIComponent(id)}`);
  }

  addSeries(url: string, overrides: Overrides = {}): Promise<Series> {
    return this.request('POST', '/series', { url, overrides });
  }

  editSeries(id: string, patch: Patch): Promise<Series> {
    return this.request('PATCH', `/series/${encodeURIComponent(id)}`, patch);
  }

  async removeSeries(id: string): Promise<void> {
    await this.request('DELETE', `/series/${encodeURIComponent(id)}`);
  }

  refreshSeries(id: string): Promise<RefreshResult> {
    return this.request('POST', `/series/${encodeURIComponent(id)}/refresh`);
  }

  markReadUpTo(id: string, upTo: number): Promise<Series> {
    return this.request('POST', `/series/${encodeURIComponent(id)}/read`, { up_to: upTo });
  }

  markRead(id: string, keys: string[], read: boolean): Promise<Series> {
    return this.request('POST', `/series/${encodeURIComponent(id)}/read`, { keys, read });
  }

  markSeen(id: string): Promise<Series> {
    return this.request('POST', `/series/${encodeURIComponent(id)}/seen`);
  }

  async listRecipes(): Promise<RecipeInfo[]> {
    return (await this.request<{ recipes: RecipeInfo[] }>('GET', '/recipes')).recipes;
  }

  /** The stored JSON text of a recipe. */
  async getRecipe(domain: string): Promise<string> {
    return (await this.send('GET', `/recipes/${encodeURIComponent(domain)}`)).text();
  }

  async saveRecipe(domain: string, json: string): Promise<void> {
    await this.request('PUT', `/recipes/${encodeURIComponent(domain)}`, undefined, json);
  }

  async deleteRecipe(domain: string): Promise<void> {
    await this.request('DELETE', `/recipes/${encodeURIComponent(domain)}`);
  }

  /** Tries a recipe (its JSON text, saved or not) on a page. */
  testRecipe(domain: string, json: string, url: string): Promise<RecipeTest> {
    return this.request('POST', `/recipes/${encodeURIComponent(domain)}/test`, {
      url,
      recipe: JSON.parse(json) as unknown,
    });
  }

  /** Asks the language model for a recipe from a series page (RF-54); nothing is saved. */
  generateRecipe(domain: string, url: string): Promise<GeneratedRecipe> {
    return this.request('POST', `/recipes/${encodeURIComponent(domain)}/generate`, { url });
  }

  /** Starts (or returns the running) "refresh all" job (RF-21). */
  refreshAll(): Promise<Job> {
    return this.request('POST', '/refresh');
  }

  /** Progress of a background job. */
  job(id: string): Promise<Job> {
    return this.request('GET', `/jobs/${encodeURIComponent(id)}`);
  }

  /** Moves a series to a new address (RF-08); force skips the same-work check. */
  changeSource(id: string, url: string, force = false): Promise<Series> {
    return this.request('POST', `/series/${encodeURIComponent(id)}/source`, { url, force });
  }

  /** Downloads the series' current cover from its site again. */
  refreshCover(id: string): Promise<Series> {
    return this.request('POST', `/series/${encodeURIComponent(id)}/cover/refresh`);
  }

  /** Uploads an image file as the series cover (kept by refreshes). */
  uploadCover(id: string, image: Blob): Promise<Series> {
    const type = image.type || 'application/octet-stream';
    return this.request('PUT', `/series/${encodeURIComponent(id)}/cover`, undefined, image, type);
  }

  /** Downloads the whole library as an export file (RF-43). */
  async exportLibrary(): Promise<{ blob: Blob; filename: string }> {
    const res = await this.send('GET', '/export');
    const disposition = res.headers.get('Content-Disposition') ?? '';
    const filename = /filename="([^"]+)"/.exec(disposition)?.[1] ?? 'leftoff-export.json';
    return { blob: await res.blob(), filename };
  }

  /** Merges an export file (its JSON text) into the library. */
  importLibrary(json: string): Promise<ImportReport> {
    return this.request('POST', '/import', undefined, json);
  }

  /** Stops the desktop app's local server (app mode only). */
  async quitApp(): Promise<void> {
    await this.request('POST', '/app/quit');
  }

  /** URL of the local cover, usable in <img src> (token as query). A
   *  version changes the URL so a replaced cover is shown right away. */
  coverUrl(id: string, version = 0): string {
    const params = new URLSearchParams();
    const token = this.token();
    if (token) params.set('token', token);
    if (version) params.set('v', String(version));
    const qs = params.toString();
    return `${BASE}/covers/${encodeURIComponent(id)}` + (qs ? `?${qs}` : '');
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
    raw?: BodyInit,
    rawType = 'application/json',
  ): Promise<T> {
    const res = await this.send(method, path, body, raw, rawType);
    if (res.status === 204) return undefined as T;
    const text = await res.text();
    return (text ? JSON.parse(text) : undefined) as T;
  }

  /** Sends a request (JSON body, or a raw body of rawType) and throws ApiError on failure. */
  private async send(
    method: string,
    path: string,
    body?: unknown,
    raw?: BodyInit,
    rawType = 'application/json',
  ): Promise<Response> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    const token = this.token();
    if (token) headers.Authorization = `Bearer ${token}`;
    const payload = raw ?? (body === undefined ? undefined : JSON.stringify(body));
    if (payload !== undefined)
      headers['Content-Type'] = raw === undefined ? 'application/json' : rawType;

    let res: Response;
    try {
      res = await this.fetchFn(BASE + path, { method, headers, body: payload });
    } catch (err) {
      if (err instanceof TypeError) this.onReachable?.(false); // network failure
      throw err;
    }
    // The service worker answered with a saved copy: the server is not reachable.
    this.onReachable?.(res.headers.get('X-MT-Offline') !== '1');
    if (!res.ok) {
      const text = await res.text();
      let err: ErrorBody['error'] | undefined;
      try {
        err = (JSON.parse(text) as ErrorBody).error;
      } catch {
        err = undefined; // not a JSON error body (e.g. a proxy page)
      }
      throw new ApiError(res.status, err?.code ?? 'http_error', err?.message ?? res.statusText);
    }
    return res;
  }
}
