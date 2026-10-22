import { describe, expect, it, vi } from 'vitest';
import { ApiClient, ApiError } from './client';

function mockFetch(status: number, body?: unknown) {
  return vi.fn<typeof fetch>(
    async () => new Response(body === undefined ? null : JSON.stringify(body), { status }),
  );
}

describe('ApiClient', () => {
  it('sends JSON with the bearer token', async () => {
    const fetchFn = mockFetch(201, { id: '1' });
    const api = new ApiClient(() => 'tok', fetchFn);
    await api.addSeries('https://a.org/x', { title: 'T' });
    const [url, init = {}] = fetchFn.mock.calls[0] ?? [];
    expect(url).toBe('api/v1/series');
    expect(init.method).toBe('POST');
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer tok');
    expect(JSON.parse(init.body as string)).toEqual({
      url: 'https://a.org/x',
      overrides: { title: 'T' },
    });
  });

  it('builds list queries without empty values', async () => {
    const fetchFn = mockFetch(200, { series: [] });
    await new ApiClient(undefined, fetchFn).listSeries({
      q: 'solo',
      has_new: true,
      status: undefined,
      sort: 'title',
    });
    expect(fetchFn.mock.calls[0]?.[0]).toBe('api/v1/series?q=solo&has_new=true&sort=title');
  });

  it('throws ApiError with the server code', async () => {
    const api = new ApiClient(
      undefined,
      mockFetch(409, { error: { code: 'duplicate', message: 'exists' } }),
    );
    await expect(api.addSeries('u')).rejects.toEqual(new ApiError(409, 'duplicate', 'exists'));
  });

  it('handles 204 and errors without a JSON body', async () => {
    await expect(
      new ApiClient(undefined, mockFetch(204)).removeSeries('1'),
    ).resolves.toBeUndefined();
    const err = await new ApiClient(undefined, mockFetch(502))
      .getSeries('1')
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).code).toBe('http_error');
  });

  it('exports with the file name from Content-Disposition and imports raw JSON', async () => {
    const fetchFn = vi.fn<typeof fetch>(async (_url, init) =>
      init?.method === 'GET'
        ? new Response('{"series":[]}', {
            headers: { 'Content-Disposition': 'attachment; filename="leftoff-1.json"' },
          })
        : new Response('{"added":1,"merged":0,"chapters_read":0}'),
    );
    const api = new ApiClient(undefined, fetchFn);
    const { blob, filename } = await api.exportLibrary();
    expect(filename).toBe('leftoff-1.json');
    expect(await blob.text()).toBe('{"series":[]}');
    expect(await api.importLibrary('{"x":1}')).toEqual({ added: 1, merged: 0, chapters_read: 0 });
    expect(fetchFn.mock.calls[1]?.[1]?.body).toBe('{"x":1}');
  });

  it('sends recipes as raw JSON and tests them inline', async () => {
    const fetchFn = vi.fn<typeof fetch>(async (_url, init) =>
      init?.method === 'GET' ? new Response('{"schema":1}') : new Response('{}'),
    );
    const api = new ApiClient(undefined, fetchFn);
    expect(await api.getRecipe('a.org')).toBe('{"schema":1}');
    await api.saveRecipe('a.org', '{"schema":1}');
    await api.testRecipe('a.org', '{"schema":1}', 'https://a.org/x');
    await api.deleteRecipe('a.org');
    const calls = fetchFn.mock.calls.map(([url, init = {}]) =>
      `${init.method} ${String(url)} ${String(init.body ?? '')}`.trim(),
    );
    expect(calls).toEqual([
      'GET api/v1/recipes/a.org',
      'PUT api/v1/recipes/a.org {"schema":1}',
      'POST api/v1/recipes/a.org/test {"url":"https://a.org/x","recipe":{"schema":1}}',
      'DELETE api/v1/recipes/a.org',
    ]);
  });

  it('puts the token in cover URLs', () => {
    expect(new ApiClient(() => '').coverUrl('A B')).toBe('api/v1/covers/A%20B');
    expect(new ApiClient(() => 't&x').coverUrl('1')).toBe('api/v1/covers/1?token=t%26x');
    expect(new ApiClient(() => '').coverUrl('1', 42)).toBe('api/v1/covers/1?v=42');
  });

  it('calls each endpoint with the right method and path', async () => {
    const fetchFn = mockFetch(200, {});
    const api = new ApiClient(undefined, fetchFn);
    await api.health();
    await api.preview('u');
    await api.getSeries('1');
    await api.editSeries('1', { notes: 'n' });
    await api.refreshSeries('1');
    await api.markReadUpTo('1', 10);
    await api.markRead('1', ['3'], false);
    await api.markSeen('1');
    await api.refreshCover('1');
    await api.uploadCover('1', new Blob(['x'], { type: 'image/png' }));
    await api.quitApp();
    const calls = fetchFn.mock.calls.map((c) => {
      const [url, init = {}] = c;
      return `${init.method} ${String(url)} ${String(init.body ?? '')}`.trim();
    });
    expect(calls).toEqual([
      'GET api/v1/health',
      'POST api/v1/extract/preview {"url":"u"}',
      'GET api/v1/series/1',
      'PATCH api/v1/series/1 {"notes":"n"}',
      'POST api/v1/series/1/refresh',
      'POST api/v1/series/1/read {"up_to":10}',
      'POST api/v1/series/1/read {"keys":["3"],"read":false}',
      'POST api/v1/series/1/seen',
      'POST api/v1/series/1/cover/refresh',
      'PUT api/v1/series/1/cover [object Blob]',
      'POST api/v1/app/quit',
    ]);
  });

  it('reports the server as unreachable when the service worker answers with a saved copy', async () => {
    const seen: boolean[] = [];
    for (const offline of [true, false]) {
      const headers = offline ? { 'X-MT-Offline': '1' } : undefined;
      const api = new ApiClient(
        undefined,
        async () => new Response('{"series":[]}', { status: 200, headers }),
      );
      api.onReachable = (ok) => seen.push(ok);
      await api.listSeries();
    }
    expect(seen).toEqual([false, true]);
  });
});
