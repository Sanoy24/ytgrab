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

test('offers to split a video with chapters', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await expect(page.locator('#split-row')).toBeHidden();
  await page.locator('#url').fill('https://youtu.be/aBcDeFgHiJk');
  await expect(page.locator('#inspect')).toContainText('12 chapters');
  await page.getByLabel('Also save each of the 12 chapters as its own file').check();
  await page.locator('#submit').click();
  await expect(page.locator('#queue .job-meta').first()).toContainText('split into chapters');
  await expect(page.locator('#split-row')).toBeHidden();
});

test('chooses how files are named', async ({ page }) => {
  await page.goto('/?fixture=empty#settings');
  await expect(page.locator('#pref-names-example')).toContainText('Me at the zoo [jNQXAC9IVRw] 720p.mp4');
  await page.getByLabel('File names').selectOption('channel-folder');
  await expect(page.locator('#toast')).toContainText('named by a folder per channel');
  await expect(page.locator('#pref-names-example')).toContainText('jawed / Me at the zoo');
});

test('watches a channel and manages the watch', async ({ page }) => {
  await page.goto('/?fixture=default#watching');
  await expect(page.locator('.watch')).toHaveCount(2);
  await expect(page.getByRole('button', { name: 'Watching, 1 channel or playlist' })).toBeVisible();
  await page.locator('#watch-url').fill('https://www.youtube.com/watch?v=dQw4w9WgXcQ');
  await page.getByRole('button', { name: 'Start watching' }).click();
  await expect(page.locator('#watch-error')).toContainText('channel');
  await page.locator('#watch-url').fill('https://www.youtube.com/@ChromeDevs');
  await page.locator('#watch-backfill').selectOption('5');
  await page.getByRole('button', { name: 'Start watching' }).click();
  await expect(page.locator('#toast')).toContainText('Watching ChromeDevs. Added 5 videos');
  await expect(page.locator('.watch')).toHaveCount(3);
  const card = page.locator('.watch', { hasText: 'Google for Developers' });
  await card.getByRole('button', { name: /^Check now/ }).click();
  await expect(page.locator('#toast')).toContainText('No new videos from Google for Developers.');
  await card.getByRole('button', { name: /^Pause watching/ }).click();
  await expect(card.locator('.watch-meta')).toContainText('paused');
  page.once('dialog', (d) => d.accept());
  await card.getByRole('button', { name: /^Stop watching/ }).click();
  await expect(page.locator('.watch')).toHaveCount(2);
});

test('marks or removes sponsor segments', async ({ page }) => {
  await page.goto('/?fixture=empty#settings');
  const select = page.getByLabel('Sponsor segments');
  await expect(select).toHaveValue('off');
  await select.selectOption('remove');
  await expect(page.locator('#toast')).toContainText('cut out of new downloads');
});

test('filters a watch and checks it on its own schedule', async ({ page }) => {
  await page.goto('/?fixture=empty#watching');
  await page.locator('#watch-url').fill('https://www.youtube.com/@ChromeDevs');
  await page.getByText('Filters and how often to check').click();
  await page.locator('#watch-min').selectOption('2');
  await page.locator('#watch-keywords').fill('devtools, lighthouse');
  await page.locator('#watch-interval').selectOption('24');
  await page.getByRole('button', { name: 'Start watching' }).click();
  await expect(page.locator('.watch-meta')).toContainText('2+ min · titles with “devtools, lighthouse” · daily');
});

test('sets a download window and says when queued downloads will start', async ({ page }) => {
  await page.goto('/?fixture=default#settings');
  await page.locator('#pref-window-start').selectOption('1');
  await expect(page.locator('#toast')).toContainText('only between 01:00 and 07:00');
  await page.getByRole('button', { name: /^Download/ }).click();
  await expect(page.locator('#window-note')).toContainText('Outside your download window');
  await page.getByRole('button', { name: 'Settings' }).click();
  await page.locator('#pref-window-start').selectOption('');
  await expect(page.locator('#toast')).toContainText('any time');
  await page.getByRole('button', { name: /^Download/ }).click();
  await expect(page.locator('#window-note')).toBeHidden();
});

test('searches YouTube from the link field', async ({ page }) => {
  await page.goto('/?fixture=empty');
  const input = page.locator('#url');
  await input.fill('me at the zoo');
  await expect(page.locator('#search-hint')).toBeVisible();
  await input.press('Enter');
  await expect(page.locator('.search-result')).toHaveCount(3);
  await page.locator('.search-result', { hasText: 'Me at the zoo' }).click();
  await expect(input).toHaveValue('https://www.youtube.com/watch?v=jNQXAC9IVRw');
  await expect(page.locator('#search-results')).toBeHidden();
  await expect(page.locator('#inspect')).toContainText('full walkthrough'); // the sample inspection
  await input.fill('nothing at all');
  await input.press('Enter');
  await expect(page.locator('#search-results')).toContainText('Nothing found');
});

test('evens out loudness of converted audio', async ({ page }) => {
  await page.goto('/?fixture=empty#settings');
  const toggle = page.getByLabel('Even out loudness');
  await expect(toggle).not.toBeChecked();
  await toggle.check();
  await expect(page.locator('#toast')).toContainText('evened out in loudness');
});

test('accepts an X post link', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await page.locator('#url').fill('https://x.com/OpenAIDevs/status/2105708732323909827?s=20');
  await expect(page.locator('#url-error')).toBeHidden();
  await expect(page.locator('#search-hint')).toBeHidden();
  await expect(page.locator('#inspect')).toContainText('full walkthrough'); // the sample inspection
  await page.locator('#submit').click();
  await expect(page.locator('#queue .job')).toHaveCount(1);
});

test('clears the library, optionally deleting the files', async ({ page }) => {
  await page.goto('/?fixture=default#library');
  await expect(page.locator('#history .job')).toHaveCount(5);
  await page.locator('#clear-history').click();
  const dialog = page.getByRole('dialog', { name: 'Clear the library?' });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Clear list' })).toBeVisible();
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  await expect(dialog).toBeHidden();
  await expect(page.locator('#history .job')).toHaveCount(5);

  await page.locator('#clear-history').click();
  await dialog.getByLabel(/Also delete/).check();
  await dialog.getByRole('button', { name: /^Clear and delete files?$/ }).click();
  await expect(page.locator('#toast')).toContainText(/Removed 5 entries and deleted \d+ files?\./);
  await expect(page.locator('#history .job')).toHaveCount(0);
});

test('lists every video of an X post with several', async ({ page }) => {
  await page.goto('/?fixture=empty');
  await page.locator('#url').fill('https://x.com/CTVJLaidlaw/status/1600649710662213632');
  await expect(page.locator('#playlist-title')).toHaveText('This post has 2 videos');
  await expect(page.locator('#playlist-legend')).toHaveText('Videos to add');
  await expect(page.locator('#playlist-items li')).toHaveText([/Video 1\s*1:53/, /Video 2\s*1:42/]);
  await page.locator('#playlist-items li').nth(0).getByRole('checkbox').uncheck();
  await expect(page.locator('#submit')).toHaveText('Add 1 video');
  await page.locator('#submit').click();
  await expect(page.locator('#toast')).toContainText('Added 1 video');
  await expect(page.locator('#queue .job')).toHaveCount(1);

  // A link to one of the videos shows that video's formats instead.
  await page.locator('#url').fill('https://x.com/CTVJLaidlaw/status/1600649710662213632/video/2');
  await expect(page.locator('#playlist')).toBeHidden();
  await expect(page.locator('#video-formats')).toContainText('720p');
});

test('shows file sizes, sorts the library, and exports it', async ({ page }) => {
  await page.goto('/?fixture=default#library');
  const summary = page.locator('#library-summary');
  await expect(summary).toHaveText(/^\d+ files? · [\d.]+ [KMG]B on disk$/);
  await expect(page.locator('#history .job').first().locator('.job-meta')).toContainText(/MB/);

  await page.getByLabel('Sort the library').selectOption('title');
  const titles = await page.locator('#history .job-title').allInnerTexts();
  expect(titles).toEqual([...titles].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base', numeric: true })));

  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Export CSV' }).click();
  const file = await download;
  expect(file.suggestedFilename()).toMatch(/^ytgrab-library-\d{4}-\d{2}-\d{2}\.csv$/);
  const csv = (await (await import('node:fs/promises')).readFile(await file.path(), 'utf8')).replace(/^\uFEFF/, '');
  const lines = csv.trim().split('\r\n');
  expect(lines[0]).toBe('Title,Site,Link,Format,State,Size (bytes),Updated,File');
  expect(lines).toHaveLength(titles.length + 1);

  // The chosen order is remembered.
  await page.reload();
  await expect(page.getByLabel('Sort the library')).toHaveValue('title');
});
