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
  const bestAudio = [...(info.audio || [])].sort((a, b) => (b.abr ?? 0) - (a.abr ?? 0));
  const m4a = bestAudio.find((a) => a.ext === 'm4a');
  // Estimated audio added to each video row; yt-dlp picks the best audio when merging.
  const audioBytes = size(m4a || bestAudio[0] || {}) ?? 0;

  const byRes = new Map();
  for (const f of info.video || []) {
    if (!f.height) continue;
    const fps = f.fps && f.fps > 30 ? Math.round(f.fps) : 0;
    const key = `${f.height}p${fps || ''}`;
    const cur = byRes.get(key);
    const better =
      !cur || compat(f) > compat(cur) || (compat(f) === compat(cur) && (size(f) ?? 0) > (size(cur) ?? 0));
    if (better) byRes.set(key, { ...f, key, fps });
  }

  const video = [...byRes.values()]
    .sort((a, b) => b.height - a.height || b.fps - a.fps)
    .map((f) => {
      const total = size(f) != null ? size(f) + audioBytes : null;
      return {
        kind: 'video',
        id: f.format_id,
        height: f.height,
        label: `${f.height}p${f.fps ? ` ${f.fps}fps` : ''}`,
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
  const cap = { 'video-1080': 1080, 'video-720': 720 }[preset];
  if (preset === 'video-best') return video[0];
  if (cap) return video.find((v) => v.height <= cap);
  if (preset === 'audio-m4a') return audio.find((a) => a.ext === 'm4a') || audio[0];
  return null; // audio-mp3 stays a conversion preset
}
