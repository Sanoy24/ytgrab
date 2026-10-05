// Turns an inspection result (GET /api/inspect) into short, user-facing choice lists.
// YouTube offers the same resolution in several codecs; the UI shows one row per
// resolution and frame rate, preferring H.264 in MP4 because it plays everywhere.

const CODECS = [
  [/^avc1/, 'H.264'],
  [/^(vp09|vp9)/, 'VP9'],
  [/^av01/, 'AV1'],
  [/^mp4a/, 'AAC'],
  [/^opus/, 'Opus'],
];

export function codecName(codec) {
  if (!codec || codec === 'none') return '';
  const hit = CODECS.find(([re]) => re.test(codec));
  return hit ? hit[1] : codec.split('.')[0];
}

const size = (f) => f.filesize ?? f.filesize_approx ?? null;
const compat = (f) => (/^avc1/.test(f.vcodec) && f.ext === 'mp4' ? 1 : 0);

export function formatSize(bytes) {
  if (bytes == null) return '';
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  while (bytes >= 1000 && i < units.length - 1) (bytes /= 1000), i++;
  return `${bytes.toFixed(bytes < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}

export function formatDuration(seconds) {
  if (seconds == null) return '';
  const s = Math.round(seconds);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const pad = (n) => String(n).padStart(2, '0');
  return h ? `${h}:${pad(m)}:${pad(s % 60)}` : `${m}:${pad(s % 60)}`;
}

// Returns { video: Choice[], audio: Choice[] }, each Choice being
// { kind, id, label, detail, height?, ext? }, sorted best first.
export function groupFormats(info) {
  // YouTube also lists "-drc" (dynamic range compressed) copies of audio streams; show
  // the original and keep a DRC copy only when it is the sole version.
  const ids = new Set((info.audio || []).map((a) => a.format_id));
  const bestAudio = (info.audio || [])
    .filter((a) => !(a.format_id.endsWith('-drc') && ids.has(a.format_id.slice(0, -4))))
    .sort((a, b) => (b.abr ?? 0) - (a.abr ?? 0));
  const m4a = bestAudio.find((a) => a.ext === 'm4a');
  // Estimated audio added to each video row; yt-dlp picks the best audio when merging.
  // X's video files already contain sound.
  const audioBytes = info.site === 'x' ? 0 : size(m4a || bestAudio[0] || {}) ?? 0;

  const byRes = new Map();
  for (const f of info.video || []) {
    if (!f.height) continue;
    // Quality is the shorter side, so a portrait 1080x1920 video is "1080p".
    const res = f.width ? Math.min(f.width, f.height) : f.height;
    const fps = f.fps && f.fps > 30 ? Math.round(f.fps) : 0;
    const key = `${res}p${fps || ''}`;
    const cur = byRes.get(key);
    const better =
      !cur || compat(f) > compat(cur) || (compat(f) === compat(cur) && (size(f) ?? 0) > (size(cur) ?? 0));
    if (better) byRes.set(key, { ...f, key, fps, res });
  }

  const video = [...byRes.values()]
    .sort((a, b) => b.res - a.res || b.fps - a.fps)
    .map((f) => {
      const total = size(f) != null ? size(f) + audioBytes : null;
      return {
        kind: 'video',
        id: f.format_id,
        height: f.res,
        label: `${f.res}p${f.fps ? ` ${f.fps}fps` : ''}${f.width && f.width < f.height ? ' · portrait' : ''}`,
        detail: [codecName(f.vcodec), total != null ? `≈ ${formatSize(total)}` : '']
          .filter(Boolean)
          .join(' · '),
      };
    });

  const seen = new Set();
  const audio = [];
  for (const a of bestAudio) {
    const abr = a.abr ? Math.round(a.abr) : null;
    const key = `${a.acodec}-${abr}`;
    if (seen.has(key)) continue;
    seen.add(key);
    audio.push({
      kind: 'audio',
      id: a.format_id,
      ext: a.ext,
      label: `${codecName(a.acodec) || a.ext.toUpperCase()}${abr ? ` · ${abr} kbps` : ''}`,
      detail: [`.${a.ext} file`, size(a) != null ? formatSize(size(a)) : '']
        .filter(Boolean)
        .join(' · '),
    });
  }

  return { video, audio };
}

// Picks the format row that best matches a quick preset, so a choice made
// before the list loaded carries over.
export function matchPreset(preset, { video, audio }) {
  const cap = { 'video-1080': 1080, 'video-720': 720, 'video-480': 480, 'video-360': 360 }[preset];
  if (preset === 'video-best') return video[0];
  if (cap) return video.find((v) => v.height <= cap);
  if (preset === 'audio-m4a') return audio.find((a) => a.ext === 'm4a') || audio[0];
  return null; // MP3, Opus, FLAC, and WAV stay conversion presets
}

// Reads "75", "1:15", or "1:02:30" (seconds may have a decimal part) as seconds.
// Returns null for an empty field and NaN for anything else.
export function parseTime(text) {
  const value = text.trim();
  if (!value) return null;
  if (!/^\d+(:\d{1,2}){0,2}(\.\d+)?$/.test(value)) return NaN;
  const parts = value.split(':').map(Number);
  if (parts.slice(1).some((n) => n >= 60)) return NaN;
  return parts.reduce((total, n) => total * 60 + n, 0);
}

// Turns the clip fields into { start, end } seconds, null for the whole video, or
// { error } explaining what to fix. duration (seconds) is used when known.
export function readSection(startText, endText, duration) {
  const start = parseTime(startText);
  const end = parseTime(endText);
  if (Number.isNaN(start) || Number.isNaN(end)) {
    return { error: 'Write times as minutes:seconds, like 1:05, or hours:minutes:seconds.' };
  }
  if (start === null && end === null) return null;
  const from = start ?? 0;
  if (duration && from >= duration) {
    return { error: `The start is after the video ends (${formatDuration(duration)}).` };
  }
  let to = end ?? duration;
  if (to == null) return { error: 'Enter an end time.' };
  if (duration && to > duration) to = duration;
  if (to - from < 1) return { error: 'The end must be at least 1 second after the start.' };
  return { start: from, end: to };
}
