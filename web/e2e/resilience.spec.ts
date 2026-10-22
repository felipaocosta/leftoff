import { expect, test } from '@playwright/test';
import { addByAPI, ELECEED, reset } from './helpers';

test.beforeEach(async ({ request }) => {
  await reset(request);
  await addByAPI(request, ELECEED);
});

test('a read mark the server refuses is rolled back', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('link', { name: /Eleceed/ }).click();
  await page.route('**/api/v1/series/*/read', (route) =>
    route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: '{"error":{"code":"internal","message":"internal error"}}',
    }),
  );
  const toggle = page.getByRole('checkbox', { name: /^Lido: / }).first();
  await toggle.check({ force: true }).catch(() => {}); // it flips back on its own
  await expect(toggle).not.toBeChecked();
  await expect(page.getByRole('alert')).toBeVisible();
  await page.unroute('**/api/v1/series/*/read');
  await page.reload();
  await expect(page.getByText(/Lido até o cap\. — de 423/)).toBeVisible(); // nothing was saved
});

test('losing the server shows a notice that goes away when it is back', async ({
  page,
  context,
}) => {
  await page.goto('/');
  await expect(page.getByRole('link', { name: /Eleceed/ })).toBeVisible();
  await page.waitForFunction(() => navigator.serviceWorker?.controller != null);
  await page.reload(); // a second visit: the service worker keeps copies now
  await expect(page.getByRole('link', { name: /Eleceed/ })).toBeVisible();
  await context.setOffline(true);
  await page.reload(); // the service worker answers with the copy it kept (RNF-06)…
  await expect(page.getByRole('link', { name: /Eleceed/ })).toBeVisible();
  // …and the user is told it is not live.
  await expect(page.getByText('Sem conexão com o servidor. Tentando de novo…')).toBeVisible();
  await context.setOffline(false);
  await page.getByRole('button', { name: 'Tentar de novo' }).click();
  await expect(page.getByText('Sem conexão com o servidor. Tentando de novo…')).toBeHidden();
});
