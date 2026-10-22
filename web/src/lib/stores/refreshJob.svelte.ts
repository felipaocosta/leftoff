import { api } from '../api';
import type { Job } from '../api/types';

const POLL_MS = 1000;

// The running "refresh all" job (RF-21). It lives outside the library page
// so progress continues while the user navigates.
class RefreshJob {
  job = $state<Job | null>(null);
  error = $state<unknown>(null);
  private listeners = new Set<(job: Job) => void>();

  get running(): boolean {
    return this.job?.status === 'running';
  }

  /** Starts the job (or joins the running one) and polls until done. */
  async start(): Promise<void> {
    this.error = null;
    try {
      this.job = await api.refreshAll();
      while (this.job.status === 'running') {
        await new Promise((r) => setTimeout(r, POLL_MS));
        this.job = await api.job(this.job.id);
      }
      for (const fn of this.listeners) fn(this.job);
    } catch (err) {
      this.error = err;
    }
  }

  /** Calls fn when a job finishes; returns the unsubscribe function. */
  onDone(fn: (job: Job) => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }
}

export const refreshJob = new RefreshJob();
