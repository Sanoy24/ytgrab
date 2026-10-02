import { expect, test } from '@playwright/test';

// These tests drive the real page with its built-in sample data (?fixture=...), so they
// need no server, tools, or network.

test('shows the queue with live progress and the history', async ({ page }) => {
  await page.goto('/?fixture=default');
  await expect(page.locator('#queue .job')).toHaveCount(3);
  await expect(page.locator('#history .job')).toHaveCount(5);
  await expect(page).toHaveTitle(/downloading · ytgrab/);
  await page.getByRole('button', { name: 'Failed / cancelled' }).click();
  await expect(page.locator('#history .badge[data-state="completed"]')).toHaveCount(0);
});

test('asks for a download folder on first run', async ({ page }) => {
  await page.goto('/?fixture=first-run');
  const card = page.locator('#setup-folder');
  await expect(card).toBeVisible();
  await page.locator('#url').fill('https://youtu.be/aBcDeFgHiJk');
  await page.locator('#submit').click();
  await expect(page.locator('#setup-error')).toHaveText('Choose a download folder first.');
  await page.locator('#setup-pick').click(); // the sample picker cancels the first time
  await expect(page.locator('#setup-pick')).toHaveText('Choose folder…');
  await expect(card).toBeVisible();
  await page.locator('#setup-pick').click();
  await expect(card).toBeHidden();
  await expect(page.locator('#dir-path')).toHaveText(/Videos/);
});

test('replaces presets with real formats and adds the chosen one', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await page.locator('#url').fill('https://youtu.be/aBcDeFgHiJk');
  await expect(page.locator('#inspect')).toContainText('full walkthrough');
  await expect(page.locator('#video-formats .preset')).toHaveCount(10);
  await page.locator('#video-formats label', { hasText: '720p 60fps' }).click();
  await page.locator('#submit').click();
  await expect(page.locator('#queue .job-meta').first()).toContainText('Video · 720p');
  await expect(page.locator('#url')).toHaveValue('');
});

test('pages through a long playlist and caps one confirmation at 200', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await page.locator('#url').fill('https://www.youtube.com/playlist?list=PLbig0000000000');
  await expect(page.locator('#playlist-meta')).toHaveText('Showing 50 of 230 videos');
  for (let shown = 100; shown <= 200; shown += 50) {
    await page.locator('#playlist-more').click();
    await expect(page.locator('#playlist-items li')).toHaveCount(shown);
  }
  await page.locator('#playlist-more').click();
  await expect(page.locator('#playlist-items li')).toHaveCount(230);
  await expect(page.locator('#submit')).toHaveText('Choose up to 200 videos');
  await expect(page.locator('#submit')).toBeDisabled();
  const boxes = page.locator('#playlist-items input');
  for (let i = 0; i < 30; i++) await boxes.nth(i).uncheck();
  await expect(page.locator('#submit')).toHaveText('Add 200 videos');
});

test('shows the YouTube pause with a countdown and resumes on request', async ({ page }) => {
  await page.goto('/?fixture=cooldown');
  await expect(page.locator('#pause-banner')).toBeVisible();
  await expect(page.locator('#pause-text')).toHaveText(/resume automatically in 1\d:\d\d/);
  await page.getByRole('button', { name: 'Resume now' }).click();
  await expect(page.locator('#pause-banner')).toBeHidden();
});

test('asks before deleting a downloaded file', async ({ page }) => {
  await page.goto('/?fixture=default');
  const done = page.locator('#history .job', { hasText: 'Lecture 1' });
  await done.getByRole('button', { name: /^Remove/ }).click();
  await expect(done.getByRole('button', { name: /Delete file too/ })).toBeVisible();
  await done.getByRole('button', { name: /^Keep/ }).click();
  await expect(done.getByRole('button', { name: /^Show in folder/ })).toBeVisible();
});

test('fits a phone screen without sideways scrolling', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 800 });
  await page.goto('/?fixture=degraded');
  await expect(page.locator('#health-label')).toHaveText('2 tools missing');
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

test('turns Start with Windows on and off from More settings', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await page.getByText('More settings').click();
  const box = page.getByLabel('Start with Windows');
  await expect(box).not.toBeChecked();
  await box.check();
  await expect(page.locator('#toast')).toContainText('start in the tray when you sign in');
  await expect(box).toBeChecked();
  await box.uncheck();
  await expect(page.locator('#toast')).toContainText("won't start when you sign in");
});

test('offers a YTGrab update with the command for this install', async ({ page }) => {
  await page.goto('/?fixture=default');
  const banner = page.locator('#update-banner');
  await expect(banner).toContainText('YTGrab 1.8.0 is available (you have 1.7.0).');
  await expect(page.locator('#update-command')).toHaveText('scoop update ytgrab');
  await page.getByRole('button', { name: 'Not now' }).click();
  await expect(banner).toBeHidden();
  await page.reload();
  await expect(page.locator('#queue .job')).toHaveCount(3);
  await expect(banner).toBeHidden(); // dismissed for this version
});
