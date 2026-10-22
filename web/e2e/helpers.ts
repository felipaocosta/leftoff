import { expect, type APIRequestContext, type Page } from '@playwright/test';
import { controlURL } from '../playwright.config';

export const GRAND_FINALE = 'https://www.mgeko.cc/manga/grand-finale/';
export const ELECEED = 'https://www.mangaread.org/manga/eleceed/';
export const HUNTER = 'https://www.mangaread.org/manga/hunter-academys-strongest-battle-god/';
export const ROLIASCAN = 'https://roliascan.com/manga/healing-at-the-academy-convenience-store/';

/**
 * Empties the library and takes chapter 50 of Grand Finale back. The
 * server keeps the pages it read for 10 minutes for adding (SPEC §5.8),
 * so a test that counts Grand Finale chapters after another test
 * released chapter 50 may still see it: only flow.spec.ts releases, and
 * other tests count chapters of other series.
 */
export async function reset(request: APIRequestContext): Promise<void> {
  const res = await request.get('/api/v1/series');
  const { series } = (await res.json()) as { series: { id: string }[] };
  for (const s of series) await request.delete(`/api/v1/series/${s.id}`);
  await request.post(`${controlURL}/reset`);
}

/** The site publishes chapter 50 of Grand Finale. */
export async function release(request: APIRequestContext): Promise<void> {
  await request.post(`${controlURL}/release`);
}

/** Adds a series through the API (for tests about other screens). */
export async function addByAPI(request: APIRequestContext, url: string): Promise<string> {
  const res = await request.post('/api/v1/series', { data: { url } });
  expect(res.status()).toBe(201);
  return ((await res.json()) as { id: string }).id;
}

/** Adds a series through the Add screen, as a user would. */
export async function addByUI(page: Page, url: string): Promise<void> {
  await page.goto('/#/add');
  await page.getByLabel('Link da obra').fill(url);
  await page.getByRole('button', { name: 'Buscar' }).click();
  await expect(page.getByText('Confira os dados antes de salvar')).toBeVisible();
  await page.getByRole('button', { name: 'Salvar na biblioteca' }).click();
}
