import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

// Automated accessibility checks (WCAG 2.1 A and AA) for every view, in both themes.
for (const scheme of ['light', 'dark']) {
  for (const view of ['download', 'library', 'watching', 'settings']) {
    test(`${view} view has no accessibility violations (${scheme})`, async ({ page }) => {
      await page.emulateMedia({ colorScheme: scheme });
      await page.goto(`/?fixture=default#${view}`);
      await expect(page.locator('#history .job').first()).toBeAttached();
      const results = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
        .exclude('.job-thumb img') // thumbnails come from YouTube and may not load in CI
        .analyze();
      const summary = results.violations.map((v) => `${v.id}: ${v.nodes.length} × ${v.nodes[0].target.join(' ')} — ${v.help}`);
      expect(summary).toEqual([]);
    });
  }
}

test('keyboard users can skip to the link field, and buttons read naturally', async ({ page }) => {
  await page.goto('/?fixture=default');
  await expect(page.locator('#queue .job').first()).toBeAttached();
  await page.keyboard.press('Tab');
  await expect(page.getByRole('link', { name: 'Skip to the link field' })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('#url')).toBeFocused();
  await expect(page.getByRole('button', { name: 'Download, 3 in the queue' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Library, 5 downloads' })).toBeVisible();
});
