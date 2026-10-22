import { expect, test } from '@playwright/test';
import { addByAPI, ELECEED, GRAND_FINALE, HUNTER, reset } from './helpers';

test.beforeEach(async ({ request }) => {
  await reset(request);
  for (const url of [GRAND_FINALE, ELECEED, HUNTER]) await addByAPI(request, url);
});

const cards = (page: import('@playwright/test').Page) =>
  page.getByRole('main').getByRole('heading', { level: 2 });

test('search, filter and sort the library', async ({ page }) => {
  await page.goto('/');
  await expect(cards(page)).toHaveCount(3);

  await page.getByRole('searchbox', { name: 'Buscar por título' }).fill('elec');
  await expect(cards(page)).toHaveText(['Eleceed']);
  await page.getByRole('searchbox', { name: 'Buscar por título' }).fill('');
  await expect(cards(page)).toHaveCount(3);

  const filters = page.getByRole('button', { name: /^Filtros/ });
  if ((await filters.getAttribute('aria-expanded')) !== 'true') await filters.click();
  await page.getByRole('combobox', { name: 'Status' }).selectOption({ label: 'Completo' });
  await expect(cards(page)).toHaveText(['Hunter Academy’s Strongest Battle God']);
  await expect(filters).toContainText('1'); // one active filter
  await page.getByRole('combobox', { name: 'Status' }).selectOption({ index: 0 });

  await page.getByRole('combobox', { name: 'Ordenar por' }).selectOption({ label: 'Título (A–Z)' });
  await expect(cards(page)).toHaveText([
    'Eleceed',
    'Grand Finale',
    'Hunter Academy’s Strongest Battle God',
  ]);
});

test('the list view is remembered', async ({ page }) => {
  await page.goto('/');
  await page
    .getByRole('group', { name: 'Visualização' })
    .getByRole('button', { name: 'Lista' })
    .click();
  await page.reload();
  await expect(page.getByRole('button', { name: 'Lista' })).toHaveAttribute('aria-pressed', 'true');
});

test('remove a series after confirming', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('link', { name: /Eleceed/ }).click();
  page.once('dialog', (d) => void d.dismiss());
  await page.getByRole('button', { name: 'Remover' }).click();
  await expect(page.getByRole('heading', { name: 'Eleceed' })).toBeVisible(); // canceled
  page.once('dialog', (d) => void d.accept());
  await page.getByRole('button', { name: 'Remover' }).click();
  await expect(cards(page)).toHaveCount(2);
});
