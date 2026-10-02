import assert from 'node:assert/strict';
import { test } from 'node:test';

import { parseLinks, validateUrl, videoIdOf } from '../static/links.js';

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
  assert.match(validateUrl('https://vimeo.com/123').error, /Only YouTube/);
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
