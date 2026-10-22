import { connection } from '../stores/connection.svelte';
import { prefs } from '../stores/prefs.svelte';
import { ApiClient } from './client';

export { ApiError } from './client';
export type * from './types';

/** The app-wide API client, using the token from the user's preferences. */
export const api = new ApiClient(() => prefs.token);
api.onReachable = (ok) => connection.report(ok);
connection.watch(() => api.health());
