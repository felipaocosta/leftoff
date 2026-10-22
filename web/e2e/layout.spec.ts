import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { addByAPI, ELECEED, GRAND_FINALE, reset } from './helpers';

const screens = ['/', '/#/add', '/#/news', '/#/settings', '/#/recipes', '/#/about'];

test.beforeAll(async ({ request }) => {
  await reset(request);
  await addByAPI(request, GRAND_FINALE);
  await addByAPI(request, ELECEED);
});

async function firstSeries(request: APIRequestContext): Promise<string> {
  const res = await request.get('/api/v1/series');
  const { series } = (await res.json()) as { series: { id: string }[] };
  const id = series[0]?.id;
  if (!id) throw new Error('no series in the library');
  return id;
}

async function noSideScroll(page: Page, where: string) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(overflow, `${where} scrolls sideways by ${overflow}px`).toBeLessThanOrEqual(0);
}

test('no screen scrolls sideways, in light and dark', async ({ page, request }) => {
  const id = await firstSeries(request);
  for (const theme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme: theme });
    for (const path of [...screens, `/#/series/${id}`]) {
      await page.goto(path);
      await page.waitForLoadState('networkidle');
      await noSideScroll(page, `${path} (${theme})`);
    }
  }
});

test('every button and link has a name', async ({ page, request }) => {
  const id = await firstSeries(request);
  for (const path of [...screens, `/#/series/${id}`]) {
    await page.goto(path);
    await page.waitForLoadState('networkidle');
    const unnamed = await page.evaluate(() =>
      [...document.querySelectorAll('button, a[href], input:not([type=hidden]), select, textarea')]
        .filter((el) => (el as HTMLElement).offsetParent !== null || el === document.activeElement)
        .filter((el) => {
          const label =
            el.getAttribute('aria-label') ||
            el.getAttribute('title') ||
            (el as HTMLElement).innerText.trim();
          const id = el.getAttribute('id');
          const forLabel = id ? document.querySelector(`label[for="${id}"]`) : null;
          return !label && !forLabel && !el.closest('label') && !el.getAttribute('aria-labelledby');
        })
        .map((el) => el.outerHTML.slice(0, 120)),
    );
    expect(unnamed, `${path} has controls without a name`).toEqual([]);
  }
});

test('the library works with the keyboard alone', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('searchbox', { name: 'Buscar por título' }).focus();
  await page.keyboard.type('grand');
  await expect(page.getByRole('main').getByRole('heading', { level: 2 })).toHaveText([
    'Grand Finale',
  ]);
  const card = page.getByRole('link', { name: /Grand Finale/ });
  for (let i = 0; i < 30 && !(await card.evaluate((el) => el === document.activeElement)); i++) {
    await page.keyboard.press('Tab');
  }
  await expect(card).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { name: 'Grand Finale', level: 1 })).toBeVisible();
  const toggle = page.getByRole('checkbox', { name: /^Lido: / }).first();
  await toggle.focus();
  await page.keyboard.press('Space');
  await expect(toggle).toBeChecked();
});
