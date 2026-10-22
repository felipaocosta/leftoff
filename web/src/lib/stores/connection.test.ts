import { afterEach, describe, expect, it, vi } from 'vitest';
import { connection } from './connection.svelte';
import { ApiClient } from '../api/client';

describe('connection', () => {
  afterEach(() => {
    vi.useRealTimers();
    connection.report(true);
  });

  it('goes offline on a network failure and back when a probe answers', async () => {
    vi.useFakeTimers();
    let up = false;
    const fetchFn = vi.fn<typeof fetch>(async () => {
      if (!up) throw new TypeError('Failed to fetch');
      return new Response('{"status":"ok"}');
    });
    const api = new ApiClient(undefined, fetchFn);
    api.onReachable = (ok) => connection.report(ok);
    connection.watch(() => api.health());

    await expect(api.health()).rejects.toThrow(TypeError);
    expect(connection.offline).toBe(true);
    await vi.advanceTimersByTimeAsync(10_000); // a probe, still down
    expect(connection.offline).toBe(true);
    up = true;
    await vi.advanceTimersByTimeAsync(10_000);
    expect(connection.offline).toBe(false);
    const calls = fetchFn.mock.calls.length;
    await vi.advanceTimersByTimeAsync(30_000);
    expect(fetchFn.mock.calls.length).toBe(calls); // no probes while online
  });

  it('treats HTTP errors as reachable', async () => {
    const api = new ApiClient(undefined, async () => new Response('{}', { status: 500 }));
    api.onReachable = (ok) => connection.report(ok);
    await expect(api.health()).rejects.toThrow();
    expect(connection.offline).toBe(false);
  });
});
