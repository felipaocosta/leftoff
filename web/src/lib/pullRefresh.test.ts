import { afterEach, describe, expect, it, vi } from 'vitest';
import { onPullRefresh } from './pullRefresh';

function pull(): boolean {
  return window.dispatchEvent(new CustomEvent('mt-pull-refresh', { cancelable: true }));
}

describe('onPullRefresh', () => {
  const win = window as { LeftOffAndroid?: unknown };
  afterEach(() => delete win.LeftOffAndroid);

  it('cancels the event and tells the shell when the work ends', async () => {
    const pullRefreshDone = vi.fn();
    win.LeftOffAndroid = { pullRefreshDone };
    let finish = () => {};
    const handler = vi.fn(() => new Promise<void>((r) => (finish = r)));
    const off = onPullRefresh(handler);

    expect(pull()).toBe(false); // canceled: handled
    await vi.waitFor(() => expect(handler).toHaveBeenCalled());
    expect(pullRefreshDone).not.toHaveBeenCalled();
    finish();
    await vi.waitFor(() => expect(pullRefreshDone).toHaveBeenCalledOnce());
    off();
    expect(pull()).toBe(true); // nobody handles it any more
  });

  it('stops the spinner when the work fails', async () => {
    const pullRefreshDone = vi.fn();
    win.LeftOffAndroid = { pullRefreshDone };
    const off = onPullRefresh(() => Promise.reject(new Error('down')));
    pull();
    await vi.waitFor(() => expect(pullRefreshDone).toHaveBeenCalledOnce());
    off();
  });
});
