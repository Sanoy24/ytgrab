import assert from 'node:assert/strict';
import { test } from 'node:test';

import { looksLikeSearch, parseLinks, validateUrl, videoIdOf } from '../static/links.js';

test('accepts single-video links and adds a missing scheme', () => {
  for (const link of [
    'https://www.youtube.com/watch?v=dQw4w9WgXcQ',
    'https://youtu.be/dQw4w9WgXcQ',
    'https://www.youtube.com/shorts/dQw4w9WgXcQ',
    'youtube.com/watch?v=dQw4w9WgXcQ',
  ]) {
    const result = validateUrl(link);
    assert.equal(result.error, undefined, link);
    assert.match(result.url, /^https:\/\//);
  }
});

test('turns playlist links into playlist requests and refuses Mixes', () => {
  const playlist = validateUrl('https://www.youtube.com/playlist?list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2');
  assert.ok(playlist.playlistUrl);
  assert.equal(playlist.url, undefined);

  const videoInList = validateUrl('https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2');
  assert.ok(videoInList.url && videoInList.playlistUrl && videoInList.note);

  assert.match(validateUrl('https://www.youtube.com/playlist?list=RDdQw4w9WgXcQ').error, /Mixes/);
});

test('explains what is wrong with other input', () => {
  assert.match(validateUrl('').error, /Paste/);
  assert.match(validateUrl('not a link').error, /doesn't look like a link/);
  assert.match(validateUrl('https://www.dailymotion.com/video/x8abcd').error, /Only YouTube/);
  assert.match(validateUrl('ftp://youtube.com/watch?v=x').error, /http and https/);
  assert.match(validateUrl('https://www.youtube.com/@channel').error, /video or playlist/);
});

test('reads video IDs from every kind of single-video link', () => {
  for (const [link, id] of [
    ['https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=10', 'dQw4w9WgXcQ'],
    ['https://youtu.be/dQw4w9WgXcQ?si=abc', 'dQw4w9WgXcQ'],
    ['https://www.youtube.com/shorts/aBcDeFgHiJk', 'aBcDeFgHiJk'],
    ['https://m.youtube.com/live/aBcDeFgHiJk?feature=share', 'aBcDeFgHiJk'],
    ['https://www.youtube.com/watch?v=tooshort', ''],
  ]) {
    assert.equal(videoIdOf(link), id, link);
  }
});

test('splits several pasted links, dropping repeats and non-video links', () => {
  assert.equal(parseLinks('https://youtu.be/dQw4w9WgXcQ'), null);
  assert.equal(parseLinks('   '), null);
  const parsed = parseLinks(`https://youtu.be/dQw4w9WgXcQ
    youtube.com/watch?v=aBcDeFgHiJk
    https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PLx
    https://www.youtube.com/playlist?list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2
    https://vimeo.com/123 not-a-link`);
  assert.deepEqual(parsed.videos.map((v) => v.id), ['dQw4w9WgXcQ', 'aBcDeFgHiJk']);
  assert.equal(parsed.skipped, 3);
});

test('tells search words from links', () => {
  for (const words of ['lofi hip hop', 'gemma 4 explained', 'cats']) assert.equal(looksLikeSearch(words), true, words);
  for (const link of ['https://youtu.be/dQw4w9WgXcQ', 'youtube.com/watch?v=dQw4w9WgXcQ', 'www.youtube.com/@x', 'x', '', 'vimeo.com/123']) {
    assert.equal(looksLikeSearch(link), false, link);
  }
});

test('accepts X post links and rewrites them to one form', () => {
  for (const link of [
    'https://x.com/OpenAIDevs/status/2105708732323909827?s=20',
    'twitter.com/OpenAIDevs/status/2105708732323909827',
    'https://mobile.twitter.com/a/status/2105708732323909827/video/1',
  ]) {
    assert.deepEqual(validateUrl(link), { url: 'https://x.com/i/status/2105708732323909827', site: 'x', id: '2105708732323909827' }, link);
  }
  assert.match(validateUrl('https://x.com/OpenAIDevs').error, /doesn't point to a post/);
  assert.equal(looksLikeSearch('x.com/OpenAIDevs/status/2105708732323909827'), false);
});

test('links to one video of an X post with several', () => {
  assert.deepEqual(validateUrl('https://x.com/a/status/1600649710662213632/video/2'), {
    url: 'https://x.com/i/status/1600649710662213632/video/2',
    site: 'x',
    id: '1600649710662213632-2',
  });
  assert.equal(validateUrl('https://x.com/a/status/1600649710662213632/video/9').url, 'https://x.com/i/status/1600649710662213632');
});

test('several pasted X links are kept, each video once', () => {
  const { videos, skipped } = parseLinks(
    'https://x.com/a/status/1600649710662213632 twitter.com/a/status/1600649710662213632 https://x.com/a/status/1600649710662213632/video/2 https://youtu.be/jNQXAC9IVRw',
  );
  assert.deepEqual(videos.map((v) => v.id), ['1600649710662213632', '1600649710662213632-2', 'jNQXAC9IVRw']);
  assert.equal(skipped, 0);
});

test('accepts Reddit post links and rewrites them to one form', () => {
  for (const link of [
    'https://www.reddit.com/r/videos/comments/6rrwyj/that_small_heart_attack/',
    'old.reddit.com/r/videos/comments/6rrwyj/',
    'https://redd.it/6rrwyj',
  ]) {
    assert.deepEqual(validateUrl(link), { url: 'https://www.reddit.com/comments/6rrwyj', site: 'reddit', id: '6rrwyj' }, link);
  }
  assert.deepEqual(validateUrl('https://v.redd.it/zv89llsvexdz'), { url: 'https://v.redd.it/zv89llsvexdz', site: 'reddit', id: 'zv89llsvexdz' });
  assert.match(validateUrl('https://www.reddit.com/r/videos/s/AbCdEf1234').error, /share links/);
  assert.match(validateUrl('https://www.reddit.com/r/videos/').error, /doesn't point to a post/);
  assert.equal(looksLikeSearch('redd.it/6rrwyj'), false);
});

test('accepts Instagram post and reel links and rewrites them to one form', () => {
  for (const link of [
    'https://www.instagram.com/reel/Dd_8Q80veb1/?utm_source=ig_web_copy_link&stkn=NTc4MTIwNjQ2YQ==',
    'instagram.com/p/Dd_8Q80veb1',
    'https://www.instagram.com/someone/reel/Dd_8Q80veb1/',
  ]) {
    assert.deepEqual(validateUrl(link), { url: 'https://www.instagram.com/p/Dd_8Q80veb1/', site: 'instagram', id: 'Dd_8Q80veb1' }, link);
  }
  assert.deepEqual(validateUrl('https://www.instagram.com/p/BQ0eAlwhDrw/?item=3'), {
    url: 'https://www.instagram.com/p/BQ0eAlwhDrw/?item=3',
    site: 'instagram',
    id: 'BQ0eAlwhDrw.3',
  });
  assert.match(validateUrl('https://www.instagram.com/someone/').error, /doesn't point to a post/);
  assert.equal(looksLikeSearch('instagram.com/reel/Dd_8Q80veb1'), false);
});

test('accepts Vimeo links and rewrites them to the player', () => {
  for (const link of ['https://vimeo.com/76979871', 'vimeo.com/channels/staffpicks/76979871', 'https://player.vimeo.com/video/76979871']) {
    assert.deepEqual(validateUrl(link), { url: 'https://player.vimeo.com/video/76979871', site: 'vimeo', id: '76979871' }, link);
  }
  assert.equal(validateUrl('https://vimeo.com/123456789/abcdef1234').url, 'https://player.vimeo.com/video/123456789?h=abcdef1234');
  assert.match(validateUrl('https://vimeo.com/staffpicks').error, /doesn't point to a video/);
  assert.equal(looksLikeSearch('vimeo.com/76979871'), false);
});
