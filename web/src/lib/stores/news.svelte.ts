import { api } from '../api';

// How many series have chapters the user has not seen yet (RF-23). The
// count is refreshed on demand and every few minutes while the app is open.
class News {
  count = $state(0);

  async refresh(): Promise<void> {
    try {
      this.count = (await api.listSeries({ has_new: true })).length;
    } catch {
      // Offline or unauthorized: keep the last known count.
    }
  }
}

export const news = new News();
