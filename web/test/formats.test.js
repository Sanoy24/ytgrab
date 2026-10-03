import assert from 'node:assert/strict';
import { test } from 'node:test';

import { codecName, formatDuration, formatSize, groupFormats, matchPreset, parseTime, readSection } from '../static/formats.js';
import { inspection } from '../static/fixtures.js';

test('groups video by resolution and frame rate, preferring H.264', () => {
  const { video } = groupFormats(inspection);
  const labels = video.map((v) => v.label);
  assert.deepEqual(labels.slice(0, 4), ['2160p', '1440p', '1080p 60fps', '1080p']);
  assert.equal(new Set(labels).size, labels.length, 'one row per resolution');
  const p1080 = video.find((v) => v.label === '1080p');
  assert.equal(p1080.id, '137', 'H.264 MP4 chosen over VP9 and AV1');
  assert.match(p1080.detail, /^H\.264/);
});

test('lists audio by codec and bitrate and hides DRC copies', () => {
  const { audio } = groupFormats(inspection);
  assert.ok(!audio.some((a) => a.id.endsWith('-drc')));
  assert.deepEqual(audio.map((a) => a.label), ['Opus · 135 kbps', 'AAC · 130 kbps', 'Opus · 70 kbps', 'Opus · 50 kbps', 'AAC · 49 kbps']);
  const drcOnly = groupFormats({ audio: [{ format_id: '140-drc', ext: 'm4a', acodec: 'mp4a.40.2', abr: 129 }] });
  assert.equal(drcOnly.audio.length, 1, 'a DRC stream is kept when it is the only one');
});

test('maps quick presets to the closest real format', () => {
  const grouped = groupFormats(inspection);
  assert.equal(matchPreset('video-1080', grouped).label, '1080p 60fps');
  assert.equal(matchPreset('video-720', grouped).label, '720p 60fps');
  assert.equal(matchPreset('audio-m4a', grouped).id, '140');
  assert.equal(matchPreset('audio-mp3', grouped), null, 'MP3 stays a conversion');
});

test('formats sizes, durations, and codec names', () => {
  assert.equal(formatSize(426_000_000), '426 MB');
  assert.equal(formatSize(1_200_000_000), '1.2 GB');
  assert.equal(formatDuration(19), '0:19');
  assert.equal(formatDuration(3725), '1:02:05');
  assert.equal(codecName('vp09.00.10.08'), 'VP9');
  assert.equal(codecName('none'), '');
});

test('reads clip times in seconds, minutes, and hours', () => {
  assert.equal(parseTime(''), null);
  assert.equal(parseTime('75'), 75);
  assert.equal(parseTime('1:15'), 75);
  assert.equal(parseTime('1:02:30'), 3750);
  assert.equal(parseTime('0:05.5'), 5.5);
  for (const bad of ['1:75', 'abc', '1:2:3:4', '-5', '1,5']) assert.ok(Number.isNaN(parseTime(bad)), bad);
});

test('turns clip fields into a section', () => {
  assert.equal(readSection('', '', 300), null);
  assert.deepEqual(readSection('1:05', '2:30', 300), { start: 65, end: 150 });
  assert.deepEqual(readSection('', '0:10', 300), { start: 0, end: 10 });
  assert.deepEqual(readSection('4:00', '', 300), { start: 240, end: 300 }); // to the end
  assert.deepEqual(readSection('4:00', '9:00', 300), { start: 240, end: 300 }); // capped
  assert.match(readSection('6:00', '', 300).error, /after the video ends/);
  assert.match(readSection('1:00', '', undefined).error, /end time/);
  assert.match(readSection('1:00', '1:00', 300).error, /at least 1 second/);
  assert.match(readSection('1:75', '', 300).error, /minutes:seconds/);
});

test('labels portrait and X videos by their shorter side', () => {
  const { video } = groupFormats({
    site: 'x',
    video: [
      { format_id: 'http-2176', ext: 'mp4', width: 720, height: 1280, filesize_approx: 7_000_000 },
      { format_id: 'http-10368', ext: 'mp4', width: 1080, height: 1920, filesize_approx: 30_000_000 },
    ],
    audio: [],
  });
  assert.deepEqual(video.map((v) => v.label), ['1080p · portrait', '720p · portrait']);
  assert.match(video[1].detail, /≈ 7.0 MB$/); // no audio added: X files already contain it
  assert.equal(matchPreset('video-720', { video, audio: [] }).id, 'http-2176');
});
