// Android pull-to-refresh: the app shell dispatches a cancelable
// "mt-pull-refresh" event when the user pulls the page down. A screen that
// handles it cancels the event, and the shell's spinner stops when the
// screen calls pullRefreshDone. Elsewhere (a browser) the event never fires.

interface AndroidBridge {
  pullRefreshDone(): void;
}

/** Runs handler on a pull-to-refresh; returns the cleanup (for $effect). */
export function onPullRefresh(handler: () => unknown): () => void {
  const listener = (e: Event) => {
    e.preventDefault();
    const bridge = (window as { LeftOffAndroid?: Partial<AndroidBridge> }).LeftOffAndroid;
    const done = () => bridge?.pullRefreshDone?.();
    Promise.resolve().then(handler).then(done, done); // the screen shows its own errors
  };
  window.addEventListener('mt-pull-refresh', listener);
  return () => window.removeEventListener('mt-pull-refresh', listener);
}
