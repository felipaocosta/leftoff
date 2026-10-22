import { expect, test } from '@playwright/test';
import { addByAPI, ELECEED, reset } from './helpers';

test.beforeEach(async ({ request }) => reset(request));

test('export the library and import it back', async ({ page, request }) => {
  const id = await addByAPI(request, ELECEED);
  await request.post(`/api/v1/series/${id}/read`, { data: { up_to: 5 } });
  await page.goto('/#/settings');

  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Exportar biblioteca' }).click();
  const file = await (await download).path();
  await expect(page.getByText(/Exportação baixada: leftoff-/).first()).toBeVisible();

  await reset(request); // the library is gone…
  page.on('dialog', (d) => void d.accept());
  await page.getByLabel('Importar arquivo…').setInputFiles(file);
  // Shown in the panel and in a toast.
  await expect(
    page
      .getByRole('status')
      .filter({ hasText: /Importação concluída: 1 obras novas, 0 juntadas/ })
      .first(),
  ).toBeVisible();

  await page.goto('/');
  await page.getByRole('link', { name: /Eleceed/ }).click();
  await expect(page.getByText('Lido até o cap. 5 de 423 · 418 não lidos')).toBeVisible(); // …and back
});

test('the dark theme is applied and remembered', async ({ page }) => {
  await page.goto('/#/settings');
  await page.getByRole('radio', { name: 'Escuro' }).check();
  const bg = () => page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  const dark = await bg();
  await page.getByRole('radio', { name: 'Claro' }).check();
  expect(await bg()).not.toBe(dark);
  await page.getByRole('radio', { name: 'Escuro' }).check();
  await page.reload();
  await expect(page.getByRole('radio', { name: 'Escuro' })).toBeChecked();
  expect(await bg()).toBe(dark);
});

test('the UI switches to English', async ({ page }) => {
  await page.goto('/#/settings');
  await page.getByRole('radio', { name: 'English' }).check();
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
  await page.getByRole('radio', { name: 'Português (Brasil)' }).check();
  await expect(page.getByRole('heading', { name: 'Configurações' })).toBeVisible();
});
