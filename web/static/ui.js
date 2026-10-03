// Small helpers shared by the page's modules: elements, toasts, formatting, thumbnails,
// and icons. Nothing here holds app state.

export const $ = (sel) => document.querySelector(sel);

export function el(tag, props = {}, ...children) {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
}

let toastTimer;

export function toast(message, isError = false) {
  const msg = document.createElement('span');
  msg.className = 'toast';
  msg.dataset.kind = isError ? 'error' : 'ok';
  msg.textContent = message;
  $('#toast').replaceChildren(msg);
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => $('#toast').replaceChildren(), isError ? 8000 : 4000);
}

export function stateBlock(kind, title, body, action) {
  const div = document.createElement('div');
  div.className = `state${kind === 'error' ? ' state-error' : ''}`;
  const strong = document.createElement('strong');
  strong.textContent = title;
  div.append(strong, body);
  if (action) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'btn btn-small';
    b.textContent = action.label;
    b.addEventListener('click', action.onClick);
    div.append(document.createElement('br'), b);
  }
  return div;
}

export function skeleton(n) {
  return Array.from({ length: n }, () =>
    Object.assign(document.createElement('div'), { className: 'skeleton' }),
  );
}

export function formatBytes(n) {
  if (n == null) return '';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1000 && i < units.length - 1) ((n /= 1000), i++);
  return `${n.toFixed(n < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}

export function formatEta(s) {
  if (s == null) return '';
  if (s < 60) return `${s}s left`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s left`;
  return `${Math.floor(m / 60)}h ${m % 60}m left`;
}

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });

export function formatWhen(iso) {
  const diff = (new Date(iso) - Date.now()) / 1000;
  const abs = Math.abs(diff);
  if (abs < 45) return 'just now';
  if (abs < 3600) return rtf.format(Math.round(diff / 60), 'minute');
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), 'hour');
  return rtf.format(Math.round(diff / 86400), 'day');
}

// YouTube's standard preview image for a video. Loaded straight from YouTube's image
// server, without sending which page asked for it.
// Also accepts a post's preview image, which the server keeps only from X's, Reddit's, and
// Instagram's image servers (the hosts the page's Content-Security-Policy allows).
export function thumbnailUrl(videoId) {
  if (/^https:\/\/(pbs\.twimg\.com|external-preview\.redd\.it|preview\.redd\.it|[a-z0-9.-]+\.fbcdn\.net|[a-z0-9.-]+\.cdninstagram\.com)\/[^\s"'<>]+$/.test(videoId || '')) return videoId;
  return /^[A-Za-z0-9_-]{11}$/.test(videoId || '') ? `https://i.ytimg.com/vi/${videoId}/mqdefault.jpg` : '';
}

// The image stays in place but invisible until it loads: a hidden lazy image never loads.
export function setThumbnail(img, videoId) {
  const src = thumbnailUrl(videoId);
  if (img.dataset.src === src) return;
  img.dataset.src = src;
  img.classList.remove('loaded');
  if (!src) return img.removeAttribute('src');
  img.onload = () => img.classList.toggle('loaded', img.naturalWidth > 120); // YouTube's "no thumbnail" image is 120 px
  img.onerror = () => img.classList.remove('loaded');
  img.src = src;
}

// Row actions shown as icons; the removal choices stay as labelled buttons.
export const ICONS = {
  cancel: '<path d="M6 6l12 12M18 6 6 18"/>',
  retry: '<path d="M3 12a9 9 0 1 0 2.64-6.36L3 8.3"/><path d="M3 3.5v4.8h4.8"/>',
  reveal: '<path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H9l2 2h7.5A2.5 2.5 0 0 1 21 9.5v7a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 16.5z"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V6a2 2 0 0 1 2-2h8"/>',
  open: '<path d="M7 4.5v15a1 1 0 0 0 1.5.86l12-7.5a1 1 0 0 0 0-1.72l-12-7.5A1 1 0 0 0 7 4.5z"/>',
  resume: '<path d="M7 4.5v15a1 1 0 0 0 1.5.86l12-7.5a1 1 0 0 0 0-1.72l-12-7.5A1 1 0 0 0 7 4.5z"/>',
  pause: '<rect x="6" y="5" width="4" height="14" rx="1"/><rect x="14" y="5" width="4" height="14" rx="1"/>',
  top: '<path d="M12 20V6M6 12l6-6 6 6M5 3h14"/>',
  again: '<path d="M12 4v11M7 10l5 5 5-5M5 20h14"/>',
  remove: '<path d="M4 7h16M10 11v6M14 11v6M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12M9 7V4h6v3"/>',
};
