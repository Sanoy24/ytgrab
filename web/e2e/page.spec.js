import { expect, test } from '@playwright/test';

// These tests drive the real page with its built-in sample data (?fixture=...), so they
// need no server, tools, or network.

test('shows the queue with live progress and the history', async ({ page }) => {
  await page.goto('/?fixture=default');
  await expect(page.locator('#queue .job')).toHaveCount(3);
  await expect(page.locator('#history .job')).toHaveCount(5);
  await expect(page).toHaveTitle(/downloading · ytgrab/);
  await page.getByRole('button', { name: /^Library/ }).click();
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
  await page.goto('/?fixture=default#library');
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
  await page.getByRole('button', { name: 'Settings' }).click();
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

test('queues several pasted links at once', async ({ page }) => {
  await page.goto('/?fixture=empty');
  const input = page.locator('#url');
  await input.focus();
  // Paste three lines: two videos and a playlist, which is skipped.
  await page.evaluate(() => {
    const data = new DataTransfer();
    data.setData('text/plain', 'https://youtu.be/aBcDeFgHiJk\nhttps://www.youtube.com/watch?v=dQw4w9WgXcQ\nhttps://www.youtube.com/playlist?list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2\n');
    document.querySelector('#url').dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }));
  });
  await expect(input).toHaveValue(/^https:\/\/youtu\.be\/aBcDeFgHiJk https:\/\/www\.youtube/);
  await expect(page.locator('#playlist-meta')).toHaveText('2 links');
  await expect(page.locator('#playlist-note')).toContainText("1 isn't a video link");
  await expect(page.locator('#submit')).toHaveText('Add 2 videos');
  await page.locator('#submit').click();
  await expect(page.locator('#toast')).toContainText('Added 2 videos.');
  await expect(page.locator('#queue .job')).toHaveCount(2);
  await expect(input).toHaveValue('');
});

test('turns on subtitles in a chosen language', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await page.getByRole('button', { name: 'Settings' }).click();
  await expect(page.locator('#pref-subs-lang-row')).toBeHidden();
  await page.getByLabel('Subtitles', { exact: true }).selectOption('embed');
  await expect(page.locator('#toast')).toContainText('English subtitles will be added');
  await expect(page.locator('#pref-subs-lang-row')).toBeVisible();
  await page.getByLabel('Subtitle language').selectOption({ label: 'Amharic' });
  await expect(page.locator('#toast')).toContainText('Amharic subtitles will be added');
  await page.getByLabel('Subtitles', { exact: true }).selectOption('off');
  await expect(page.locator('#pref-subs-lang-row')).toBeHidden();
});

test('sets a per-download speed limit', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await page.getByRole('button', { name: 'Settings' }).click();
  const select = page.getByLabel('Speed limit (per download)');
  await expect(select).toHaveValue('0');
  await select.selectOption({ label: '2 MB/s' });
  await expect(page.locator('#toast')).toContainText('each use at most 2 MB/s');
  await select.selectOption({ label: 'No limit' });
  await expect(page.locator('#toast')).toContainText('full speed');
});

test('saves a playlist into its own folder by default', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await page.locator('#url').fill('https://www.youtube.com/playlist?list=PLbig0000000000');
  const folder = page.locator('#playlist-folder');
  await expect(page.locator('#playlist-folder-label')).toHaveText('Save in a folder named “A very long series”');
  await expect(folder).toBeChecked();
  const boxes = page.locator('#playlist-items input');
  for (let i = 2; i < 50; i++) await boxes.nth(i).uncheck();
  await page.locator('#submit').click();
  await expect(page.locator('#toast')).toContainText('Saving into the “A very long series” folder.');
  // Turned off, it stays off next time.
  await page.locator('#url').fill('https://www.youtube.com/playlist?list=PLbig0000000001');
  await expect(folder).toBeChecked();
  await folder.uncheck();
  await page.reload();
  await page.locator('#url').fill('https://www.youtube.com/playlist?list=PLbig0000000000');
  await expect(page.locator('#playlist-items li')).toHaveCount(50);
  await expect(folder).not.toBeChecked();
});

test('downloads only part of a video', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await expect(page.locator('#clip')).toBeHidden();
  await page.locator('#url').fill('https://youtu.be/aBcDeFgHiJk');
  await expect(page.locator('#inspect')).toContainText('full walkthrough');
  await page.getByText('Only part of the video').click();
  await page.locator('#clip-start').fill('1:75');
  await page.locator('#submit').click();
  await expect(page.locator('#clip-error')).toContainText('minutes:seconds');
  await page.locator('#clip-start').fill('1:05');
  await page.locator('#clip-end').fill('2:30');
  await page.locator('#submit').click();
  await expect(page.locator('#toast')).toContainText('Only 1:05–2:30.');
  await expect(page.locator('#queue .job-meta').first()).toContainText('1:05–2:30');
  await expect(page.locator('#clip')).toBeHidden();
  // Playlists don't offer it.
  await page.locator('#url').fill('https://www.youtube.com/playlist?list=PLbig0000000000');
  await expect(page.locator('#playlist-items li')).toHaveCount(50);
  await expect(page.locator('#clip')).toBeHidden();
});

test('switches between video and audio and remembers each choice', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await page.locator('#url').fill('https://youtu.be/aBcDeFgHiJk');
  await expect(page.locator('#video-formats .preset')).toHaveCount(10);
  await page.locator('#video-formats label', { hasText: '720p 60fps' }).click();
  await page.getByRole('button', { name: 'Audio only' }).click();
  await expect(page.locator('#audio-formats .preset').first()).toBeVisible();
  await expect(page.locator('#video-formats')).toBeHidden();
  await page.getByRole('button', { name: 'Video', exact: true }).click();
  await expect(page.locator('#video-formats label', { hasText: '720p 60fps' }).locator('input')).toBeChecked();
});

test('takes a link pasted anywhere on the page', async ({ page }) => {
  await page.goto('/?fixture=empty#library');
  await page.evaluate(() => {
    const data = new DataTransfer();
    data.setData('text/plain', 'https://youtu.be/aBcDeFgHiJk');
    document.body.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }));
  });
  await expect(page.locator('#view-download')).toBeVisible();
  await expect(page.locator('#url')).toHaveValue('https://youtu.be/aBcDeFgHiJk');
  await expect(page.locator('#inspect')).toContainText('full walkthrough');
});

test('takes a link handed over by the Send to YTGrab bookmark', async ({ page }) => {
  await page.goto('/?fixture=empty&url=' + encodeURIComponent('https://www.youtube.com/watch?v=aBcDeFgHiJk&t=42'));
  await expect(page.locator('#url')).toHaveValue('https://www.youtube.com/watch?v=aBcDeFgHiJk&t=42');
  await expect(page.locator('#inspect')).toContainText('full walkthrough');
  await expect(page).toHaveURL(/\?fixture=empty$/); // the address is tidied
  await expect(page.locator('#queue .job')).toHaveCount(0); // nothing is queued by itself
  await page.getByRole('button', { name: 'Settings' }).click();
  await expect(page.locator('#send-bookmark')).toHaveAttribute('href', /^javascript:/);
});

test('keeps yt-dlp up to date unless turned off', async ({ page }) => {
  await page.goto('/?fixture=empty#settings');
  const toggle = page.getByLabel('Keep yt-dlp up to date');
  await expect(toggle).toBeChecked();
  await toggle.uncheck();
  await expect(page.locator('#toast')).toContainText("won't be updated automatically");
});

test('retries every failed download and offers to download one again', async ({ page }) => {
  await page.goto('/?fixture=default#library');
  const retry = page.getByRole('button', { name: /^Retry failed/ });
  await expect(retry).toHaveText('Retry failed (3)');
  await retry.click();
  await expect(page.locator('#toast')).toContainText('Queued 3 downloads again.');
  await expect(retry).toBeHidden();
  const done = page.locator('#history .job', { hasText: 'Lecture 1' });
  await done.getByRole('button', { name: /^Download again/ }).click();
  await expect(page.locator('#view-download')).toBeVisible();
  await expect(page.locator('#url')).toHaveValue(/YE7VzlLtp-4/);
});

test('pauses, resumes, and reorders downloads', async ({ page }) => {
  await page.goto('/?fixture=default');
  const running = page.locator('#queue .job', { hasText: 'local-first download manager' });
  await running.getByRole('button', { name: /^Pause/ }).click();
  await expect(running.locator('.badge')).toHaveText('Paused');
  await expect(running.locator('.job-progress-text')).toContainText('Paused');
  await running.getByRole('button', { name: /^Resume/ }).click();
  await expect(running.locator('.badge')).not.toHaveText('Paused');
  // "Me at the zoo" is the only waiting download, so it needs no move.
  await expect(page.locator('#queue .job', { hasText: 'Me at the zoo' }).getByRole('button', { name: /^Move to top/ })).toHaveCount(0);
});

test('offers Opus, FLAC, and WAV for audio', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await page.getByRole('button', { name: 'Audio only' }).click();
  for (const name of ['M4A', 'MP3', 'Opus', 'FLAC', 'WAV']) {
    await expect(page.locator('#preset-choices .preset', { hasText: name }).first()).toBeVisible();
  }
  await page.locator('#url').fill('https://youtu.be/aBcDeFgHiJk');
  await expect(page.locator('#audio-formats .preset', { hasText: 'FLAC' })).toHaveCount(1);
  await page.locator('#audio-formats .preset', { hasText: 'FLAC' }).click();
  await page.locator('#submit').click();
  await expect(page.locator('#queue .job-meta').first()).toContainText('Audio · FLAC');
});

test('a large library stays fast and is not redrawn on every progress tick', async ({ page }) => {
  await page.goto('/?fixture=big-library#library');
  await expect(page.locator('#history .job')).toHaveCount(405);
  const mutations = await page.evaluate(async () => {
    let count = 0;
    new MutationObserver((m) => (count += m.length)).observe(document.querySelector('#history'), { subtree: true, childList: true, characterData: true, attributes: true });
    await new Promise((r) => setTimeout(r, 4000)); // the sample downloads tick every second meanwhile
    return count;
  });
  expect(mutations).toBeLessThan(400); // was over 14,000 when every row was redrawn
  await page.locator('#history-search').fill('episode 39');
  await expect(page.locator('#history .job')).toHaveCount(11); // 39 and 390–399
});
