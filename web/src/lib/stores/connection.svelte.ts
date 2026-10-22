// Whether the server answers (the local core, or the server in connected
// mode). The API client reports every request; while offline the state
// probes /health every few seconds and clears itself when it answers.

const PROBE_MS = 10_000;

class Connection {
  offline = $state(false);
  private timer: ReturnType<typeof setInterval> | undefined;
  private probe: () => Promise<unknown> = async () => undefined;

  /** Wires the client: probe is a cheap request such as api.health(). */
  watch(probe: () => Promise<unknown>): void {
    this.probe = probe;
  }

  report(reachable: boolean): void {
    if (reachable === !this.offline) return;
    this.offline = !reachable;
    clearInterval(this.timer);
    if (this.offline) this.timer = setInterval(() => void this.retry(), PROBE_MS);
  }

  /** Tries the server now (the banner's "try again"). */
  async retry(): Promise<void> {
    try {
      await this.probe(); // a success reports itself through the client
    } catch {
      // still offline; the next probe tries again
    }
  }
}

export const connection = new Connection();
