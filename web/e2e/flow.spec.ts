import { expect, test } from '@playwright/test';
import { addByUI, GRAND_FINALE, release, reset, ROLIASCAN } from './helpers';

test.beforeEach(async ({ request }) => reset(request));

test('add, read up to a chapter, get a new one and see it in News', async ({ page, request }) => {
  await addByUI(page, GRAND_FINALE);
  await expect(page.getByRole('heading', { name: 'Grand Finale' })).toBeVisible();
  await expect(page.getByText('Lido até o cap.')).toContainText('49');

  await page.getByRole('button', { name: 'Marcar todos até o capítulo 10 como lidos' }).click();
  await expect(page.getByText('Lido até o cap. 10 de 49 · 39 não lidos')).toBeVisible();
  await expect(page.getByRole('checkbox', { name: /Lido: .*10/ })).toBeChecked();

  // A reload shows what the server saved, not the optimistic state.
  await page.reload();
  await expect(page.getByText('Lido até o cap. 10 de 49 · 39 não lidos')).toBeVisible();

  await release(request);
  await page.getByRole('button', { name: 'Atualizar', exact: true }).click();
  await expect(page.getByText('1 capítulo novo.')).toBeVisible();
  await expect(page.getByText('Lido até o cap. 10 de 50 · 40 não lidos')).toBeVisible();

  await page.goto('/#/news');
  await expect(page.getByRole('link', { name: /Grand Finale/ })).toBeVisible();
  await page
    .getByRole('link', { name: /Grand Finale/ })
    .first()
    .click();
  await expect(page.getByRole('heading', { name: 'Grand Finale' })).toBeVisible();
  await expect(page.getByRole('link', { name: /^Novidades/ }).first()).toHaveText('Novidades'); // badge gone
  await page.goto('/#/news'); // opening the series cleared its "new" mark
  await expect(page.getByText(/Nenhuma novidade/)).toBeVisible();
});

// RF-04: the duplicate is refused when saving (the preview does not check).
test('a duplicate is refused on the Add screen', async ({ page }) => {
  await addByUI(page, GRAND_FINALE);
  await expect(page.getByRole('heading', { name: 'Grand Finale' })).toBeVisible();
  await page.goto('/#/add');
  await page.getByLabel('Link da obra').fill(GRAND_FINALE + '?utm_source=share');
  await page.getByRole('button', { name: 'Buscar' }).click();
  await page.getByRole('button', { name: 'Salvar na biblioteca' }).click();
  await expect(page.getByText('Essa obra já está na sua biblioteca.')).toBeVisible();
});

test('a page with few chapters asks for confirmation before saving', async ({ page }) => {
  await page.goto('/#/add');
  await page.getByLabel('Link da obra').fill(ROLIASCAN);
  await page.getByRole('button', { name: 'Buscar' }).click();
  await expect(page.getByText(/confiança da extração está baixa/)).toBeVisible();
  const save = page.getByRole('button', { name: 'Salvar na biblioteca' });
  await expect(save).toBeDisabled();
  await page.getByLabel('Conferi os dados e quero salvar mesmo assim').check();
  await expect(save).toBeEnabled();
});

test('a site that does not answer gets a clear message', async ({ page }) => {
  await page.goto('/#/add');
  await page.getByLabel('Link da obra').fill('https://down.test/manga/x');
  await page.getByRole('button', { name: 'Buscar' }).click();
  await expect(page.getByText(/O site não respondeu ou devolveu um erro/)).toBeVisible();
});
