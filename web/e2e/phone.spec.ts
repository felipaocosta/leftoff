import { expect, test } from '@playwright/test';
import { addByAPI, ELECEED, reset } from './helpers';

test.beforeEach(async ({ request }, info) => {
  test.skip(info.project.name !== 'phone', 'phone layout only');
  await reset(request);
  await addByAPI(request, ELECEED);
});

test('phone: bottom tab bar, filters folded, and the tab bar stays on screen', async ({ page }) => {
  await page.goto('/');
  const tabs = page.getByRole('navigation', { name: 'Navegação principal' });
  await expect(tabs).toBeVisible();
  const box = await tabs.boundingBox();
  const height = page.viewportSize()?.height ?? 0;
  expect((box?.y ?? 0) + (box?.height ?? 0)).toBeGreaterThan(height - 2); // at the bottom edge
  await expect(page.getByRole('button', { name: /^Filtros/ })).toHaveAttribute(
    'aria-expanded',
    'false',
  );

  await page.getByRole('link', { name: /Eleceed/ }).click();
  await page.mouse.wheel(0, 5000); // a long chapter list
  await expect(tabs).toBeInViewport();
  await expect(page.getByRole('link', { name: /^Continuar/ }).first()).toBeInViewport(); // sticky Continue
});
