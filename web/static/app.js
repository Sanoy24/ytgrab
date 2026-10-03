import { createClient, isActive } from './api.js';
import { formatDuration, groupFormats, matchPreset, readSection } from './formats.js';
import { parseLinks, validateUrl, videoIdOf } from './links.js';

const client = createClient();
const $ = (sel) => document.querySelector(sel);

const PRESET_LABELS = {
  'video-best': 'Video · best',
  'video-1080': 'Video · 1080p',
  'video-720': 'Video · 720p',
  'audio-m4a': 'Audio · M4A',
  'audio-mp3': 'Audio · MP3',
  'audio-opus': 'Audio · Opus',
  'audio-flac': 'Audio · FLAC',
  'audio-wav': 'Audio · WAV',
};

const STATE_LABELS = {
  queued: 'Queued',
  inspecting: 'Checking',
  downloading: 'Downloading',
  processing: 'Processing',
  completed: 'Done',
  failed: 'Failed',
  cancelled: 'Cancelled',
  paused: 'Paused',
};

// Extra guidance appended to server error messages; only where the server's text lacks it.
const ERROR_HINTS = {
  video_unavailable: 'Check that the video is public and the link is correct.',
};


let jobs = [];
let historyFilter = 'all';
let historyQuery = ''; // library search, lower case
const pending = new Set(); // job ids with an action in flight

// ---------- URL validation ----------


// ---------- formatting ----------

function formatBytes(n) {
  if (n == null) return '';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1000 && i < units.length - 1) ((n /= 1000), i++);
  return `${n.toFixed(n < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}

function formatEta(s) {
  if (s == null) return '';
  if (s < 60) return `${s}s left`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s left`;
  return `${Math.floor(m / 60)}h ${m % 60}m left`;
}

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
function formatWhen(iso) {
  const diff = (new Date(iso) - Date.now()) / 1000;
  const abs = Math.abs(diff);
  if (abs < 45) return 'just now';
  if (abs < 3600) return rtf.format(Math.round(diff / 60), 'minute');
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), 'hour');
  return rtf.format(Math.round(diff / 86400), 'day');
}

// YouTube's standard preview image for a video. Loaded straight from YouTube's image
// server, without sending which page asked for it.
function thumbnailUrl(videoId) {
  return /^[A-Za-z0-9_-]{11}$/.test(videoId || '') ? `https://i.ytimg.com/vi/${videoId}/mqdefault.jpg` : '';
}

// The image stays in place but invisible until it loads: a hidden lazy image never loads.
function setThumbnail(img, videoId) {
  const src = thumbnailUrl(videoId);
  if (img.dataset.src === src) return;
  img.dataset.src = src;
  img.classList.remove('loaded');
  if (!src) return img.removeAttribute('src');
  img.onload = () => img.classList.toggle('loaded', img.naturalWidth > 120); // YouTube's "no thumbnail" image is 120 px
  img.onerror = () => img.classList.remove('loaded');
  img.src = src;
}

function jobTitle(job) {
  if (job.title) return job.title;
  return job.video_id ? `Video ${job.video_id}` : job.url;
}

// ---------- rendering ----------

const template = $('#job-template');
const nodes = new Map(); // job id -> element, reused so focus survives updates

// What a row shows; a row is redrawn only when this changes. Progress ticks arrive every
// second, and redrawing hundreds of unchanged Library rows each time wastes power.
function rowKey(job) {
  return [
    job.updated_at, job.state, job.title, job.attempt, job.output_path,
    JSON.stringify(job.progress), job.priority || 0,
    formatWhen(job.updated_at), pending.has(job.id), confirmingRemove.has(job.id),
    // Move to top depends on the other waiting downloads.
    job.state === 'queued' || job.state === 'paused' ? firstWaiting(job) : '',
  ].join('|');
}

function renderJob(job) {
  let el = nodes.get(job.id);
  if (!el) {
    el = template.content.firstElementChild.cloneNode(true);
    el.dataset.id = job.id;
    nodes.set(job.id, el);
  }
  const key = rowKey(job);
  if (el.dataset.key === key) return el;
  el.dataset.key = key;
  const title = el.querySelector('.job-title');
  title.textContent = jobTitle(job);
  title.title = job.title || job.url;
  setThumbnail(el.querySelector('.job-thumb img'), job.video_id);

  const badge = el.querySelector('.badge');
  badge.dataset.state = job.state;
  badge.textContent = STATE_LABELS[job.state] || job.state;

  const format = PRESET_LABELS[job.preset] || job.format?.label || job.preset || 'Custom format';
  const meta = [format, formatWhen(job.updated_at)];
  if (job.section) meta.splice(1, 0, `${formatDuration(job.section.start)}–${formatDuration(job.section.end)}`);
  if (job.split_chapters) meta.splice(1, 0, 'split into chapters');
  if (job.attempt > 1) meta.push(`attempt ${job.attempt}`);
  el.querySelector('.job-meta').textContent = meta.join(' · ');

  renderProgress(el, job);

  const path = el.querySelector('.job-path');
  path.hidden = !(job.state === 'completed' && job.output_path);
  path.textContent = job.output_path || '';
  path.title = job.output_path || '';

  const err = el.querySelector('.job-error');
  err.hidden = !(job.state === 'failed' && job.error);
  if (job.error) {
    err.replaceChildren();
    const strong = document.createElement('strong');
    strong.textContent = 'Error: ';
    err.append(strong, job.error.message);
    const hint = ERROR_HINTS[job.error.code];
    if (hint) err.append(` ${hint}`);
  }

  renderActions(el.querySelector('.job-actions'), job);
  return el;
}

function renderProgress(el, job) {
  const box = el.querySelector('.job-progress');
  const bar = box.querySelector('progress');
  const text = box.querySelector('.job-progress-text');
  const showStates = ['downloading', 'processing', 'inspecting'];
  const pausedWithProgress = job.state === 'paused' && job.progress?.downloaded_bytes;
  box.hidden = !showStates.includes(job.state) && !pausedWithProgress;
  if (box.hidden) return;
  if (job.state === 'paused') {
    const p = job.progress;
    const pct = p.total_bytes ? Math.min(100, (p.downloaded_bytes / p.total_bytes) * 100) : null;
    if (pct == null) bar.removeAttribute('value');
    else bar.value = pct;
    text.textContent = ['Paused', pct != null ? `${Math.floor(pct)}%` : '', p.total_bytes ? `${formatBytes(p.downloaded_bytes)} of ${formatBytes(p.total_bytes)}` : `${formatBytes(p.downloaded_bytes)} downloaded`]
      .filter(Boolean)
      .join(' · ');
    box.dataset.paused = 'true';
    return;
  }
  delete box.dataset.paused;

  const p = job.progress || {};
  // Merged downloads fetch video, then audio; naming the part keeps the second 0% from
  // looking like a restart.
  const part = { video: 'Video', audio: 'Audio' }[p.stream] || '';
  const label = jobTitle(job);
  bar.setAttribute('aria-label', `Progress for ${label}`);
  if (job.state === 'processing') {
    bar.removeAttribute('value'); // indeterminate
    text.textContent = 'Merging and finishing up…';
  } else if (!p.downloaded_bytes && p.total_bytes == null) {
    bar.removeAttribute('value');
    text.textContent = 'Starting…';
  } else if (job.state === 'inspecting' || p.total_bytes == null) {
    bar.removeAttribute('value');
    text.textContent = [
      part,
      `${formatBytes(p.downloaded_bytes)} downloaded`,
      p.speed_bps ? `${formatBytes(p.speed_bps)}/s` : '',
      'size unknown',
    ]
      .filter(Boolean)
      .join(' · ');
  } else {
    const pct = Math.min(100, (p.downloaded_bytes / p.total_bytes) * 100);
    bar.value = pct;
    text.textContent = [
      part,
      `${Math.floor(pct)}%`,
      `${formatBytes(p.downloaded_bytes)} of ${formatBytes(p.total_bytes)}`,
      p.speed_bps ? `${formatBytes(p.speed_bps)}/s` : '',
      formatEta(p.eta_seconds),
    ]
      .filter(Boolean)
      .join(' · ');
  }
}

// Finished downloads whose Remove button was pressed; they show the removal choices.
const confirmingRemove = new Set();

// Row actions shown as icons; the removal choices stay as labelled buttons.
const ICONS = {
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

// The waiting download that will start next needs no "move to top".
function firstWaiting(job) {
  const waiting = jobs
    .filter((j) => j.state === 'queued' || j.state === 'paused')
    .sort((a, b) => (b.priority || 0) - (a.priority || 0) || a.created_at.localeCompare(b.created_at));
  return waiting[0]?.id === job.id;
}

function renderActions(box, job) {
  const want = [];
  if (job.state === 'paused') want.push(['resume', 'Resume', '']);
  else if (isActive(job.state)) want.push(['pause', 'Pause', '']);
  if ((job.state === 'queued' || job.state === 'paused') && !firstWaiting(job)) want.push(['top', 'Move to top', '']);
  if (isActive(job.state)) want.push(['cancel', 'Cancel', 'btn-danger']);
  if (job.state === 'failed' || job.state === 'cancelled') want.push(['retry', 'Retry', ''], ['remove', 'Remove', '']);
  if (job.state === 'completed' && confirmingRemove.has(job.id)) {
    want.push(['remove-list', 'Remove from list', ''], ['remove-file', 'Delete file too', 'btn-danger'], ['remove-keep', 'Keep', '']);
  } else if (job.state === 'completed') {
    if (job.output_path) want.push(['open', 'Play', ''], ['reveal', 'Show in folder', ''], ['copy', 'Copy path', '']);
    want.push(['again', 'Download again', '']);
    want.push(['remove', 'Remove', '']);
  }

  const key = want.map((w) => w[0]).join(',');
  if (box.dataset.key !== key) {
    box.dataset.key = key;
    box.replaceChildren(
      ...want.map(([action, label, cls]) => {
        const b = document.createElement('button');
        b.type = 'button';
        b.dataset.action = action;
        b.setAttribute('aria-label', `${label}: ${jobTitle(job)}`);
        if (ICONS[action]) {
          b.className = 'act';
          b.title = label;
          b.innerHTML = `<svg viewBox="0 0 24 24" aria-hidden="true">${ICONS[action]}</svg>`;
        } else {
          b.className = `btn btn-small ${cls}`.trim();
          b.textContent = label;
        }
        return b;
      }),
    );
  }
  for (const b of box.querySelectorAll('button')) b.disabled = pending.has(job.id);
}

function stateBlock(kind, title, body, action) {
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

function skeleton(n) {
  return Array.from({ length: n }, () =>
    Object.assign(document.createElement('div'), { className: 'skeleton' }),
  );
}

function fillList(container, list, empty) {
  container.setAttribute('aria-busy', 'false');
  if (!list.length) {
    container.replaceChildren(empty);
    return;
  }
  // Re-append in order; existing nodes are moved, not recreated.
  const els = list.map(renderJob);
  if (
    els.some((el, i) => container.children[i] !== el) ||
    container.children.length !== els.length
  ) {
    container.replaceChildren(...els);
  }
}

const RUN_ORDER = { processing: 0, downloading: 1, inspecting: 2, queued: 3, paused: 3 };

// ---------- finish notifications and tab title ----------

// Notifications are opt-in (the browser asks for permission) and only fire while the
// YTGrab tab is in the background. The choice is remembered in this browser.
const NOTIFY_KEY = 'ytgrab.notify';
const UPDATE_DISMISSED_KEY = 'ytgrab.updateDismissed';
const PLAYLIST_FOLDER_KEY = 'ytgrab.playlistFolder';

// Saving a playlist into its own folder is on unless the user turned it off.
function playlistFolderPreferred() {
  try {
    return localStorage.getItem(PLAYLIST_FOLDER_KEY) !== '0';
  } catch {
    return true;
  }
}
let notifyOn = false;
let lastStates = null; // job id -> state at the previous render

function notificationsSupported() {
  return 'Notification' in window;
}

function loadNotifyPreference() {
  try {
    notifyOn = notificationsSupported() && Notification.permission === 'granted' && localStorage.getItem(NOTIFY_KEY) === '1';
  } catch {
    notifyOn = false;
  }
}

function renderNotifyToggle() {
  const button = $('#notify-toggle');
  button.hidden = !notificationsSupported();
  button.setAttribute('aria-pressed', String(notifyOn));
  $('#notify-label').textContent = notifyOn ? 'Notifying' : 'Notify me';
  button.title = notifyOn
    ? 'Notifications on: you get one when a download finishes while this tab is in the background'
    : 'Notify me when downloads finish while this tab is in the background';
}

async function toggleNotifications() {
  if (notifyOn) {
    notifyOn = false;
    toast('Notifications turned off.');
  } else {
    const permission = await Notification.requestPermission();
    if (permission !== 'granted') {
      toast('Notifications are blocked for this page. Allow them in your browser settings to use this.', true);
      return;
    }
    notifyOn = true;
    toast("You'll get a notification when a download finishes while YTGrab is in the background.");
  }
  try {
    localStorage.setItem(NOTIFY_KEY, notifyOn ? '1' : '0');
  } catch {
    // Storage unavailable (private window): the choice lasts for this page only.
  }
  renderNotifyToggle();
}

function notify(title, body, tag) {
  try {
    const n = new Notification(title, { body, tag });
    n.onclick = () => {
      window.focus();
      n.close();
    };
  } catch {
    // Some browsers only allow notifications from a service worker; skip quietly.
  }
}

// Compares this render's job states with the last one and announces finished downloads.
function announceFinished() {
  const previous = lastStates;
  lastStates = new Map(jobs.map((j) => [j.id, j.state]));
  if (previous) {
    const ended = jobs.filter((j) => isActive(previous.get(j.id) || '') && !isActive(j.state) && j.state !== 'cancelled');
    if (ended.length) {
      $('#announcer').textContent = ended
        .map((j) => `${j.state === 'completed' ? 'Download finished' : 'Download failed'}: ${jobTitle(j)}.`)
        .join(' ');
    }
  }
  if (!previous || !notifyOn || !document.hidden) return;
  for (const job of jobs) {
    const before = previous.get(job.id);
    if (!before || !isActive(before)) continue;
    if (job.state === 'completed') notify('Download finished', jobTitle(job), job.id);
    else if (job.state === 'failed') notify('Download failed', `${jobTitle(job)}: ${job.error?.message || ''}`.trim(), job.id);
  }
}

// The tab title shows progress, so it can be followed from another tab.
function updateTitle() {
  const running = jobs.filter((j) => j.state === 'downloading' || j.state === 'processing');
  const queued = jobs.filter((j) => j.state === 'queued').length;
  let status = '';
  if (client.pausedUntil && new Date(client.pausedUntil) > new Date()) status = 'Paused';
  else if (running.length === 1) {
    const p = running[0].progress;
    status = p?.total_bytes ? `↓ ${Math.floor((p.downloaded_bytes / p.total_bytes) * 100)}%` : '↓ Downloading';
  } else if (running.length > 1) status = `↓ ${running.length} downloading`;
  else if (queued) status = `${queued} queued`;
  document.title = status ? `${status} · ytgrab` : 'ytgrab';
}

// ---------- history ----------

async function clearHistory() {
  const finished = jobs.filter((j) => !isActive(j.state)).length;
  if (!finished) {
    toast('The history is already empty.');
    return;
  }
  if (!confirm(`Remove all ${finished} finished, failed, and cancelled downloads from the list? Downloaded files are kept.`)) return;
  try {
    const { removed } = await client.clearHistory();
    jobs = jobs.filter((j) => isActive(j.state));
    render();
    toast(`Removed ${removed} entr${removed === 1 ? 'y' : 'ies'}. Your files are kept.`);
  } catch (err) {
    toast(err.message, true);
  }
}

// Queues every failed and cancelled download again; each resumes from its partial file.
async function retryFailed(button) {
  const failed = jobs.filter((j) => j.state === 'failed' || j.state === 'cancelled');
  if (!failed.length) return;
  button.disabled = true;
  let queued = 0;
  for (const job of failed) {
    try {
      await client.retryJob(job.id);
      queued++;
    } catch {
      // Already queued again elsewhere, or removed: skip it.
    }
  }
  if (!client.isFixture) jobs = await client.listJobs();
  button.disabled = false;
  render();
  toast(queued === 1 ? 'Queued 1 download again.' : `Queued ${queued} downloads again.`);
}

// ---------- YouTube pause ----------

// While YouTube limits this network the server pauses downloads and format checks;
// the banner counts down to the automatic resume.
let pauseTimer = null;

function renderPause() {
  const until = client.pausedUntil ? new Date(client.pausedUntil).getTime() : 0;
  const remaining = until - Date.now();
  const banner = $('#pause-banner');
  if (remaining <= 0) {
    banner.hidden = true;
    clearInterval(pauseTimer);
    pauseTimer = null;
    return;
  }
  const minutes = Math.floor(remaining / 60_000);
  const seconds = String(Math.floor((remaining % 60_000) / 1000)).padStart(2, '0');
  $('#pause-text').textContent = `Downloads resume automatically in ${minutes}:${seconds}.`;
  banner.hidden = false;
  pauseTimer ??= setInterval(renderPause, 1000);
}

async function resumeNow(button) {
  button.disabled = true;
  try {
    await client.resume();
    renderPause();
    toast('Resuming downloads.');
  } catch (err) {
    toast(err.message, true);
  } finally {
    button.disabled = false;
  }
}

function render() {
  renderPause();
  announceFinished();
  updateTitle();
  const queue = jobs
    .filter((j) => isActive(j.state))
    .sort(
      // Waiting downloads in the order they will start: moved-to-top first, then oldest.
      (a, b) =>
        RUN_ORDER[a.state] - RUN_ORDER[b.state] ||
        (b.priority || 0) - (a.priority || 0) ||
        a.created_at.localeCompare(b.created_at),
    );
  const history = jobs
    .filter((j) => !isActive(j.state))
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  const shown = history.filter(
    (j) =>
      (historyFilter === 'all' ? true : historyFilter === 'completed' ? j.state === 'completed' : j.state !== 'completed') &&
      (!historyQuery || jobTitle(j).toLowerCase().includes(historyQuery)),
  );
  const failed = history.filter((j) => j.state === 'failed' || j.state === 'cancelled');
  $('#retry-failed').hidden = !failed.length;
  $('#retry-failed').textContent = `Retry failed (${failed.length})`;
  $('#queue-nav-count').textContent = queue.length || '';
  $('#library-nav-count').textContent = history.length || '';
  // The counts are drawn as badges; give the buttons names that read naturally.
  setNavLabel('download', 'Download', queue.length, 'in the queue');
  setNavLabel('library', 'Library', history.length, history.length === 1 ? 'download' : 'downloads');
  renderNow(queue);

  $('#queue-count').textContent = queue.length ? `(${queue.length})` : '';
  $('#history-count').textContent = history.length ? `(${history.length})` : '';

  fillList(
    $('#queue'),
    queue,
    stateBlock(
      'empty',
      'Nothing downloading',
      'Add a link and it will show up here with live progress.',
    ),
  );

  let historyEmpty;
  if (history.length && historyQuery && !shown.length)
    historyEmpty = stateBlock('empty', 'No matches', `Nothing in the library matches "${$('#history-search').value.trim()}".`);
  else if (!history.length)
    historyEmpty = stateBlock(
      'empty',
      'No downloads yet',
      'Finished, failed, and cancelled downloads are listed here.',
    );
  else if (historyFilter === 'completed')
    historyEmpty = stateBlock(
      'empty',
      'No completed downloads',
      'Completed files will appear here.',
    );
  else historyEmpty = stateBlock('empty', 'No problems', 'Nothing has failed or been cancelled.');
  fillList($('#history'), shown, historyEmpty);

  // Drop cached nodes for jobs that no longer exist.
  const ids = new Set(jobs.map((j) => j.id));
  for (const id of nodes.keys()) if (!ids.has(id)) nodes.delete(id);
}

function setNavLabel(view, name, count, noun) {
  const button = document.querySelector(`.nav-item[data-view="${view}"]`);
  button.setAttribute('aria-label', count ? `${name}, ${count} ${noun}` : name);
}

// ---------- views ----------

const VIEWS = ['download', 'library', 'settings'];
let currentView = 'download';

function showView(name, { focus = false } = {}) {
  if (!VIEWS.includes(name)) name = 'download';
  currentView = name;
  for (const v of VIEWS) $(`#view-${v}`).hidden = v !== name;
  for (const b of document.querySelectorAll('.nav-item')) {
    if (b.dataset.view === name) b.setAttribute('aria-current', 'page');
    else b.removeAttribute('aria-current');
  }
  const hash = name === 'download' ? '' : `#${name}`;
  if (location.hash !== hash) history.replaceState(null, '', hash || location.pathname + location.search);
  $('#main').scrollTop = 0;
  if (focus) $(`#view-${name} h2`)?.focus?.();
  renderNow(jobs.filter((j) => isActive(j.state)));
}

// The download in progress, shown in the sidebar while another view is open.
function renderNow(queue) {
  const box = $('#now');
  const running = queue.filter((j) => j.state === 'downloading' || j.state === 'processing');
  if (currentView === 'download' || !queue.length) {
    box.hidden = true;
    return;
  }
  box.hidden = false;
  // Prefer a download whose size is known, so the bar shows real progress.
  const first = running.find((j) => j.progress?.total_bytes) || running[0] || queue[0];
  const p = first.progress;
  const pct = first.state === 'downloading' && p?.total_bytes ? Math.min(100, (p.downloaded_bytes / p.total_bytes) * 100) : null;
  $('#now-title').textContent = jobTitle(first);
  $('#now-meta').textContent = [
    running.length ? `${running.length} downloading` : `${queue.length} queued`,
    pct != null ? `${Math.floor(pct)}%` : first.state === 'processing' ? 'finishing' : '',
  ]
    .filter(Boolean)
    .join(' · ');
  $('#now-fill').style.width = `${pct ?? (first.state === 'processing' ? 100 : 0)}%`;
}

// ---------- video / audio switch ----------

function kindOf(choice) {
  return /^(audio:|preset:audio)/.test(choice) ? 'audio' : 'video';
}

// Shows one kind's choices. With pickFirst, a choice of the other kind is replaced by the
// first one shown, so what is selected is always visible.
function setKind(kind, { pickFirst = false } = {}) {
  for (const tab of document.querySelectorAll('.kind-tab')) tab.setAttribute('aria-pressed', String(tab.dataset.kind === kind));
  for (const group of document.querySelectorAll('.presets .preset-group')) group.hidden = group.dataset.kind !== kind;
  if (pickFirst && kindOf(selectedChoice()) !== kind) {
    const shown = $('#format-choices').hidden ? $('#preset-choices') : $('#format-choices');
    const inputs = [...shown.querySelectorAll(`.preset-group[data-kind="${kind}"] input:not(:disabled)`)];
    // Back on a kind, return to the choice made there before.
    const pick = inputs.find((i) => i.value === lastChoice[kind]) || inputs[0];
    if (pick) pick.checked = true;
  }
}

// The last choice made on each side of the switch.
const lastChoice = {};
document.addEventListener('change', (e) => {
  if (e.target.name !== 'choice') return;
  lastChoice[kindOf(e.target.value)] = e.target.value;
  if (e.isTrusted) formatTouched = true;
});

function syncKind() {
  setKind(kindOf(selectedChoice()));
}

// ---------- send to YTGrab ----------

// The bookmark opens this page (in one reused tab) with the YouTube page's address in
// ?url=. The page only fills the link field; nothing is queued until Add is pressed.
function setupSendBookmark() {
  const target = `${location.origin}/?url=`;
  const code = `javascript:(()=>{window.open(${JSON.stringify(target)}+encodeURIComponent(location.href),'ytgrab')})()`;
  const link = $('#send-bookmark');
  link.href = code;
  link.addEventListener('click', (e) => {
    e.preventDefault();
    toast('Drag this button to your bookmarks bar, then click it on a YouTube page.');
  });
}

// A link handed over by the bookmark (or any ?url= address) goes into the link field.
function takeLinkFromAddress() {
  const params = new URLSearchParams(location.search);
  const handed = params.get('url');
  if (!handed) return;
  params.delete('url');
  const rest = params.toString();
  history.replaceState(null, '', `${location.pathname}${rest ? `?${rest}` : ''}${location.hash}`);
  takeLinks(handed);
}

// ---------- paste or drop a link anywhere ----------

function takeLinks(text) {
  showView('download');
  const input = $('#url');
  input.value = oneLine(text);
  setUrlError('');
  input.dispatchEvent(new InputEvent('input', { inputType: 'insertFromPaste' }));
  input.focus();
}

function setupPasteAndDrop() {
  document.addEventListener('paste', (e) => {
    if (e.target.closest?.('input, textarea, select, [contenteditable]')) return;
    const text = e.clipboardData?.getData('text') || '';
    if (!text.trim()) return;
    e.preventDefault();
    takeLinks(text);
  });
  let depth = 0;
  const carriesText = (e) => [...(e.dataTransfer?.types || [])].some((t) => t === 'text/uri-list' || t === 'text/plain');
  const setDragging = (on) => {
    document.body.classList.toggle('dragging', on);
    $('#drop-overlay').hidden = !on;
  };
  document.addEventListener('dragenter', (e) => {
    if (!carriesText(e)) return;
    depth++;
    setDragging(true);
  });
  document.addEventListener('dragleave', () => {
    depth = Math.max(0, depth - 1);
    if (!depth) setDragging(false);
  });
  document.addEventListener('dragover', (e) => {
    if (carriesText(e)) e.preventDefault();
  });
  document.addEventListener('drop', (e) => {
    depth = 0;
    setDragging(false);
    const text = e.dataTransfer?.getData('text/uri-list') || e.dataTransfer?.getData('text/plain') || '';
    if (!text.trim()) return;
    e.preventDefault();
    takeLinks(text.split(/\r?\n/).filter((l) => l && !l.startsWith('#')).join(' '));
  });
}

function renderLoadError(err) {
  const unsupported = err.code === 'not_available';
  const retry = unsupported ? null : { label: 'Try again', onClick: loadJobs };
  const block = () =>
    unsupported
      ? stateBlock('empty', "Downloads aren't available yet", err.message)
      : stateBlock('error', "Couldn't load downloads", err.message, retry);
  for (const sel of ['#queue', '#history']) {
    const box = $(sel);
    box.setAttribute('aria-busy', 'false');
    box.replaceChildren(block());
  }
}

// ---------- health ----------

// "ffmpeg version 8.0.1-full_build…" -> "8.0.1"; "v24.4.1" -> "24.4.1".
function shortVersion(v) {
  return v?.match(/\d+(?:\.\d+)+/)?.[0] || v || '';
}

function toolList(dependencies) {
  const ul = document.createElement('ul');
  ul.className = 'tools';
  for (const d of dependencies) {
    const li = document.createElement('li');
    li.dataset.available = String(d.available);
    const name = document.createElement('code');
    name.textContent = d.name;
    const state = document.createElement('span');
    state.className = 'tool-state';
    state.textContent = d.available
      ? shortVersion(d.version) || 'found'
      : d.required
        ? 'missing'
        : 'missing (optional)';
    const msg = document.createElement('span');
    msg.className = 'tool-msg';
    // "Available." adds nothing next to a version; keep messages that carry advice.
    msg.textContent = d.available && /^available\.?$/i.test(d.message || '') ? '' : d.message || '';
    if (d.path) li.title = d.path;
    li.append(name, state, msg);
    if (d.name === 'yt-dlp' && client.updateYtdlp) {
      // yt-dlp needs updating when YouTube changes; offer it in place.
      const label = !d.available ? 'Install' : d.outdated ? 'Update' : 'Check for update';
      const button = el('button', { type: 'button', className: 'btn btn-small tool-action', textContent: label });
      button.addEventListener('click', () => updateYtdlp(button));
      li.append(button);
      if (d.outdated) li.dataset.outdated = 'true';
    }
    ul.append(li);
  }
  return ul;
}

async function updateYtdlp(button) {
  const original = button.textContent;
  button.disabled = true;
  button.textContent = 'Updating…';
  try {
    const result = await client.updateYtdlp();
    toast(
      result.updated
        ? `yt-dlp updated to ${result.version}.`
        : `yt-dlp ${result.version} is already the latest version.`,
    );
    await loadHealth();
  } catch (err) {
    toast(err.message, true);
    button.disabled = false;
    button.textContent = original;
  }
}

async function loadHealth() {
  const pill = $('#health');
  const label = $('#health-label');
  const banner = $('#health-detail');
  pill.hidden = false;
  try {
    const h = await client.health();
    const missingRequired = h.dependencies.filter((d) => d.required && !d.available);
    const ready = h.status === 'ready' && !missingRequired.length;
    const outdated = h.dependencies.some((d) => d.outdated);
    pill.dataset.status = !ready ? 'error' : outdated ? 'degraded' : 'ok';
    label.textContent = !ready
      ? `${missingRequired.length || 'Some'} tool${missingRequired.length === 1 ? '' : 's'} missing`
      : outdated
        ? 'yt-dlp update available'
        : 'Tools ready';

    const strong = document.createElement('strong');
    strong.textContent = ready
      ? 'All required tools were found.'
      : 'Downloads will fail until the missing tools are installed.';
    const parts = [strong, toolList(h.dependencies)];
    if (h.note) {
      const note = Object.assign(document.createElement('p'), {
        className: 'tools-note',
        textContent: h.note,
      });
      parts.push(note);
    }
    banner.className = `banner${ready ? ' banner-info' : ' banner-error'}`;
    banner.replaceChildren(...parts);
    setHealthOpen(!ready);
  } catch (err) {
    pill.dataset.status = 'error';
    label.textContent = 'Server offline';
    banner.className = 'banner banner-error';
    const strong = document.createElement('strong');
    strong.textContent = "The ytgrab server isn't responding.";
    banner.replaceChildren(strong, `${err.message} Start the app again, then reload this page.`);
    setHealthOpen(true);
  }
}

function setHealthOpen(open) {
  $('#health-detail').hidden = !open;
  $('#health').setAttribute('aria-expanded', String(open));
}

// ---------- jobs ----------

async function loadJobs() {
  for (const sel of ['#queue', '#history']) {
    $(sel).setAttribute('aria-busy', 'true');
    $(sel).replaceChildren(...skeleton(2));
  }
  try {
    jobs = await client.listJobs();
    render();
  } catch (err) {
    renderLoadError(err);
  }
}

async function runAction(id, action) {
  const job = jobs.find((j) => j.id === id);
  if (!job) return;
  if (action === 'copy') {
    try {
      await navigator.clipboard.writeText(job.output_path);
      toast('File path copied.');
    } catch {
      toast("Couldn't copy. Select the path text and copy it manually.", true);
    }
    return;
  }
  if (action === 'open') {
    try {
      await client.openJob(id);
    } catch (err) {
      toast(err.message, true);
    }
    return;
  }
  if (action === 'again') {
    takeLinks(job.url);
    toast('Choose a format, then add it again.');
    return;
  }
  if (action === 'reveal') {
    try {
      await client.revealJob(id);
    } catch (err) {
      toast(err.message, true);
    }
    return;
  }
  if (action === 'remove' && job.state === 'completed') {
    confirmingRemove.add(id);
    render();
    return;
  }
  if (action === 'remove-keep') {
    confirmingRemove.delete(id);
    render();
    return;
  }
  if (action === 'remove' || action === 'remove-list' || action === 'remove-file') {
    const deleteFile = action === 'remove-file';
    pending.add(id);
    render();
    try {
      await client.deleteJob(id, deleteFile);
      jobs = jobs.filter((j) => j.id !== id);
      toast(deleteFile ? 'File deleted and removed from the list.' : 'Removed from the list.');
    } catch (err) {
      toast(err.message, true);
    } finally {
      pending.delete(id);
      confirmingRemove.delete(id);
      render();
    }
    return;
  }
  pending.add(id);
  render();
  const calls = {
    cancel: [() => client.cancelJob(id), 'Download cancelled.'],
    retry: [() => client.retryJob(id), 'Download queued again.'],
    pause: [() => client.pauseJob(id), 'Paused. Resume continues from where it stopped.'],
    resume: [() => client.resumeJob(id), 'Resumed.'],
    top: [() => client.moveToTop(id), 'Moved to the top of the queue.'],
  };
  try {
    const [call, done] = calls[action] || calls.retry;
    await call();
    if (!client.isFixture) jobs = await client.listJobs();
    toast(done);
  } catch (err) {
    toast(err.message, true);
  } finally {
    pending.delete(id);
    render();
  }
}

// ---------- toast ----------

let toastTimer;
function toast(message, isError = false) {
  const msg = document.createElement('span');
  msg.className = 'toast';
  msg.dataset.kind = isError ? 'error' : 'ok';
  msg.textContent = message;
  $('#toast').replaceChildren(msg);
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => $('#toast').replaceChildren(), isError ? 8000 : 4000);
}

// ---------- form ----------

function setUrlError(message) {
  const input = $('#url');
  const out = $('#url-error');
  out.textContent = message;
  out.hidden = !message;
  input.setAttribute('aria-invalid', message ? 'true' : 'false');
}

// The clip fields apply to one video at a time, never to playlists or pasted lists.
function updateClipVisibility() {
  const single = !playlist && Boolean(validateUrl($('#url').value).url);
  $('#clip').hidden = !single;
}

function setClipError(message) {
  $('#clip-error').textContent = message;
  $('#clip-error').hidden = !message;
}

// Offered only for an inspected video that has chapters.
function renderSplitOption(chapters) {
  $('#split-row').hidden = !chapters;
  $('#split-label').textContent = `Also save each of the ${chapters} chapters as its own file`;
  if (!chapters) $('#split-chapters').checked = false;
}

function resetClip() {
  renderSplitOption(0);
  $('#clip-start').value = '';
  $('#clip-end').value = '';
  $('#clip').open = false;
  setClipError('');
}

// Returns the section to download, null for the whole video, or false after showing why
// the times can't be used.
function clipSection(url) {
  if ($('#clip').hidden) return null;
  const result = readSection($('#clip-start').value, $('#clip-end').value, cachedInspection(url)?.duration);
  if (result?.error) {
    $('#clip').open = true;
    setClipError(result.error);
    $('#clip-start').focus();
    return false;
  }
  setClipError('');
  return result;
}

async function onSubmit(e) {
  e.preventDefault();
  if (!requireFolder()) return;
  const several = parseLinks($('#url').value);
  if (several && !playlist?.batch) enterBatch(several);
  if (playlist) {
    if (playlist.list) submitPlaylist();
    return;
  }
  const input = $('#url');
  const result = validateUrl(input.value);
  if (!result.error && !result.url) {
    // Never queue a playlist without showing what it contains first.
    enterPlaylist(result.playlistUrl);
    return;
  }
  if (result.error) {
    setUrlError(result.error);
    input.focus();
    return;
  }
  setUrlError('');
  const section = clipSection(result.url);
  if (section === false) return;
  const [kind, value] = String(new FormData(e.target).get('choice')).split(':');
  const body = kind === 'preset' ? { preset: value } : { format: { kind, id: value } };
  if (section && $('#split-chapters').checked) {
    setClipError('Choose part of the video or splitting it into chapters, not both.');
    return;
  }
  if (section) body.section = section;
  else if ($('#split-chapters').checked) body.split_chapters = true;
  const button = $('#submit');
  button.disabled = true;
  button.textContent = 'Adding…';
  try {
    try {
      await client.createJob({ url: result.url, ...body });
    } catch (err) {
      // The server's inspection expired: check again and retry once with the same choice.
      if (err.code !== 'inspection_required' || !body.format) throw err;
      inspections.delete(result.url);
      inspectedUrl = '';
      const grouped = await inspect(result.url);
      const choice = `${body.format.kind}:${body.format.id}`;
      if (!grouped || selectedChoice() !== choice) {
        throw Object.assign(new Error('Formats changed since the last check. Choose one and add again.'), {
          code: 'inspection_required',
        });
      }
      await client.createJob({ url: result.url, ...body });
    }
    if (!client.isFixture) jobs = await client.listJobs();
    input.value = '';
    resetFormats();
    resetClip();
    updateClipVisibility();
    const clipped = section ? ` Only ${formatDuration(section.start)}–${formatDuration(section.end)}.` : '';
    toast(result.note ? `Added.${clipped} ${result.note}` : `Added to the queue.${clipped}`);
    render();
  } catch (err) {
    if (err.code === 'invalid_url' || err.code === 'duplicate_job') setUrlError(err.message);
    else toast(err.message, true);
  } finally {
    button.disabled = false;
    button.textContent = 'Add to queue';
  }
}

// ---------- format inspection ----------

// Formats are fetched as soon as a valid video link is entered. The quick presets stay
// usable meanwhile and remain the fallback when inspection fails.
// url -> { grouped, at }. The server trusts an inspection for 10 minutes when creating a
// format job; expiring sooner here avoids offering choices it would reject.
const inspections = new Map();
const INSPECTION_TTL_MS = 9 * 60_000;
let inspectedUrl = '';
let inspectCtl = null;
let inspectTimer;

function selectedChoice() {
  return document.querySelector('input[name="choice"]:checked')?.value || 'preset:video-best';
}

function el(tag, props = {}, ...children) {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
}

function setInspectStatus(...nodes) {
  $('#inspect').replaceChildren(...nodes);
}

function showPresets(checkedValue) {
  $('#format-choices').hidden = true;
  $('#preset-choices').hidden = false;
  for (const r of $('#preset-choices').querySelectorAll('input')) r.disabled = false;
  const want = checkedValue?.startsWith('preset:') ? checkedValue : `preset:${settings?.default_preset || 'video-best'}`;
  $('#preset-choices').querySelector(`input[value="${want}"]`).checked = true;
  syncKind();
}

function resetFormats() {
  exitPlaylist();
  clearTimeout(inspectTimer);
  inspectCtl?.abort();
  inspectCtl = null;
  inspectedUrl = '';
  setInspectStatus();
  showPresets(selectedChoice());
}

function scheduleInspect(immediate = false) {
  clearTimeout(inspectTimer);
  const several = parseLinks($('#url').value);
  if (several) {
    inspectTimer = setTimeout(() => enterBatch(several), immediate ? 0 : 500);
    return;
  }
  const result = validateUrl($('#url').value);
  if (result.error) {
    if (inspectedUrl || playlist) resetFormats();
    return;
  }
  if (!result.url) {
    inspectTimer = setTimeout(() => enterPlaylist(result.playlistUrl), immediate ? 0 : 500);
    return;
  }
  if (playlist) exitPlaylist();
  if (result.url === inspectedUrl) return;
  inspectTimer = setTimeout(() => inspect(result.url), immediate ? 0 : 500);
}

function cachedInspection(url) {
  const entry = inspections.get(url);
  if (entry && Date.now() - entry.at < INSPECTION_TTL_MS) return entry.grouped;
  inspections.delete(url);
  return null;
}

// Resolves to the grouped formats, or null when inspection failed or was superseded.
async function inspect(url) {
  inspectCtl?.abort();
  const ctl = (inspectCtl = new AbortController());
  inspectedUrl = url;

  const cached = cachedInspection(url);
  if (cached) {
    showFormats(cached, selectedChoice());
    return cached;
  }

  setInspectStatus(el('span', { className: 'spinner' }), 'Checking available formats…');
  try {
    const info = await client.inspect(url, { signal: ctl.signal });
    const grouped = { ...groupFormats(info), title: info.title, duration: info.duration_seconds, chapters: info.chapters || 0 };
    inspections.set(url, { grouped, at: Date.now() });
    if (ctl.signal.aborted) return null;
    showFormats(grouped, selectedChoice());
    return grouped;
  } catch (err) {
    if (err.name === 'AbortError' || ctl.signal.aborted) return null;
    if (err.code === 'not_available') return setInspectStatus(), null; // server without inspection
    const parts = [
      el('span', {
        className: 'inspect-error',
        textContent: `Couldn't load formats: ${err.message}`,
      }),
      ' You can still use a standard preset.',
    ];
    if (err.code !== 'video_unavailable') {
      const retry = el('button', {
        type: 'button',
        className: 'btn-link',
        textContent: 'Try again',
      });
      retry.addEventListener('click', () => {
        inspectedUrl = '';
        inspect(url);
      });
      parts.push(' ', retry);
    }
    setInspectStatus(...parts);
  }
}

function formatRow(choice, checked) {
  const input = el('input', {
    type: 'radio',
    name: 'choice',
    value: `${choice.kind}:${choice.id}`,
    checked,
  });
  const text = el(
    'span',
    {},
    el('strong', { textContent: choice.label }),
    el('small', { textContent: choice.detail }),
  );
  return el('label', { className: 'preset' }, input, text);
}

// Conversions offered next to the video's own audio formats.
const AUDIO_CONVERSIONS = [
  { kind: 'preset', id: 'audio-mp3', label: 'MP3', detail: 'Converted, plays anywhere' },
  { kind: 'preset', id: 'audio-opus', label: 'Opus', detail: '.opus file, no quality loss' },
  { kind: 'preset', id: 'audio-flac', label: 'FLAC', detail: 'Lossless copy for editing' },
  { kind: 'preset', id: 'audio-wav', label: 'WAV', detail: 'Uncompressed, large' },
];

function showFormats(grouped, previous) {
  const { video, audio } = grouped;
  if (!video.length && !audio.length) {
    setInspectStatus('No downloadable formats were listed for this video. Using standard presets.');
    return;
  }
  // Carry a choice made before the list loaded over to the closest real format.
  const all = [...video, ...audio, ...AUDIO_CONVERSIONS].map((c) => `${c.kind}:${c.id}`);
  const [kind, value] = previous.split(':');
  const match = kind === 'preset' ? matchPreset(value, grouped) : null;
  let checked = match ? `${match.kind}:${match.id}` : previous;
  if (!all.includes(checked)) checked = all[0];

  $('#video-formats').replaceChildren(
    ...(video.length
      ? video.map((c) => formatRow(c, `video:${c.id}` === checked))
      : [el('p', { className: 'hint', textContent: 'No video formats available.' })]),
  );
  $('#audio-formats').replaceChildren(
    ...[...audio, ...AUDIO_CONVERSIONS].map((c) => formatRow(c, `${c.kind}:${c.id}` === checked)),
  );

  for (const r of $('#preset-choices').querySelectorAll('input')) r.disabled = true;
  $('#preset-choices').hidden = true;
  $('#format-choices').hidden = false;
  syncKind();

  renderSplitOption(grouped.chapters);
  const meta = [
    formatDuration(grouped.duration),
    grouped.chapters ? `${grouped.chapters} chapters` : '',
    `${video.length} video · ${audio.length} audio options`,
  ]
    .filter(Boolean)
    .join(' · ');
  const parts = [
    el('strong', { className: 'inspect-title', textContent: grouped.title || 'Video' }),
    el('small', { textContent: meta }),
  ];
  const { url, playlistUrl } = validateUrl($('#url').value);
  if (playlistUrl) {
    const whole = el('button', { type: 'button', className: 'btn-link', textContent: 'Download the whole playlist instead' });
    whole.addEventListener('click', () => enterPlaylist(playlistUrl));
    parts.push(el('br'), whole);
  }
  const thumb = el('img', { className: 'inspect-thumb', alt: '', referrerPolicy: 'no-referrer' });
  setThumbnail(thumb, url ? videoIdOf(url) : '');
  setInspectStatus(thumb, el('div', { className: 'inspect-text' }, ...parts));
}

// ---------- playlists ----------

// A playlist link lists its videos for review; nothing is queued until the user presses
// the Add button, whose label states how many videos it will add.
let playlist = null; // { url, list }
let playlistCtl = null;

function playlistIds() {
  return [...document.querySelectorAll('#playlist-items input:checked')].map((i) => i.value);
}

function updatePlaylistCount() {
  const button = $('#submit');
  if (!playlist?.list) {
    button.textContent = 'Add to queue';
    button.disabled = false;
    return;
  }
  const n = playlistIds().length;
  const total = playlist.list.entries.length;
  const tooMany = n > MAX_PLAYLIST_JOBS;
  button.textContent = tooMany
    ? `Choose up to ${MAX_PLAYLIST_JOBS} videos`
    : n === 1
      ? 'Add 1 video'
      : `Add ${n} videos`;
  button.disabled = n === 0 || tooMany;
  const all = $('#playlist-all');
  all.checked = n === total;
  all.indeterminate = n > 0 && n < total;
}

async function enterPlaylist(url) {
  if (playlist?.url === url) return;
  clearTimeout(inspectTimer);
  inspectCtl?.abort();
  inspectedUrl = '';
  setInspectStatus();
  showPresets(selectedChoice());
  playlistCtl?.abort();
  const ctl = (playlistCtl = new AbortController());
  playlist = { url, list: null };
  $('#playlist').hidden = false;
  $('#clip').hidden = true;
  $('#playlist-title').replaceChildren(el('span', { className: 'spinner' }), 'Reading playlist…');
  $('#playlist-meta').textContent = '';
  $('#playlist-items').replaceChildren();
  $('#playlist-note').hidden = true;
  $('#playlist-all').parentElement.hidden = true;
  $('#submit').disabled = true;
  try {
    const list = await client.listPlaylist(url, { signal: ctl.signal, start: 1 });
    if (ctl.signal.aborted) return;
    playlist.list = list;
    renderPlaylist(list);
  } catch (err) {
    if (err.name === 'AbortError' || ctl.signal.aborted) return;
    $('#playlist-title').textContent = "Couldn't read this playlist";
    $('#playlist-meta').textContent = err.message;
    $('#submit').disabled = true;
  }
}

// Several pasted links reuse the playlist review: untick any, pick a format, add them all.
function enterBatch({ videos, skipped }) {
  const key = `links:${videos.map((v) => v.id).join(',')}:${skipped}`;
  if (playlist?.url === key) return;
  clearTimeout(inspectTimer);
  inspectCtl?.abort();
  playlistCtl?.abort();
  inspectedUrl = '';
  setInspectStatus();
  showPresets(selectedChoice());
  setUrlError(videos.length ? '' : 'None of these are links to single YouTube videos.');
  if (!videos.length) {
    if (playlist) exitPlaylist();
    return;
  }
  const list = {
    title: 'Several links',
    batch: true,
    entries: videos.map((v) => ({ video_id: v.id, title: v.url.replace(/^https:\/\/(www\.)?/, '') })),
    skippedLinks: skipped,
  };
  playlist = { url: key, list, batch: true };
  $('#playlist').hidden = false;
  $('#clip').hidden = true;
  renderPlaylist(list);
}

// One confirmation adds at most this many videos (the server enforces the same cap).
const MAX_PLAYLIST_JOBS = 200;

// playlist.list accumulates pages: entries grow, next points at the following page.
function renderPlaylist(list, append = false) {
  $('#playlist-title').textContent = list.title || 'Playlist';
  const n = list.entries.length;
  const unit = list.batch ? 'link' : 'video';
  const shown = list.next && list.total ? `Showing ${n} of ${list.total} videos` : `${n} ${unit}${n === 1 ? '' : 's'}`;
  $('#playlist-meta').textContent = shown;
  const notes = [];
  if (list.total > MAX_PLAYLIST_JOBS) notes.push(`Up to ${MAX_PLAYLIST_JOBS} videos can be added at a time.`);
  if (list.unavailable) {
    notes.push(`${list.unavailable} private or deleted video${list.unavailable === 1 ? ' is' : 's are'} skipped.`);
  }
  if (list.skippedLinks) {
    notes.push(
      `${list.skippedLinks} ${list.skippedLinks === 1 ? "isn't a video link" : "aren't video links"} and ${list.skippedLinks === 1 ? 'is' : 'are'} skipped; paste playlists on their own.`,
    );
  }
  $('#playlist-note').textContent = notes.join(' ');
  $('#playlist-note').hidden = !notes.length;
  $('#playlist-more').hidden = !list.next;
  // Pasted links have no playlist name to use as a folder.
  $('#playlist-folder-row').hidden = Boolean(list.batch);
  if (!list.batch) {
    $('#playlist-folder-label').textContent = `Save in a folder named “${list.title || 'Playlist'}”`;
    if (!append) $('#playlist-folder').checked = playlistFolderPreferred();
  }
  $('#playlist-all').parentElement.hidden = false;
  const items = append ? list.entries.slice($('#playlist-items').children.length) : list.entries;
  $('#playlist-items')[append ? 'append' : 'replaceChildren'](
    ...items.map((entry) =>
      el(
        'li',
        {},
        el(
          'label',
          {},
          el('input', { type: 'checkbox', value: entry.video_id, checked: true }),
          el('span', { textContent: entry.title || entry.video_id }),
          el('small', { textContent: formatDuration(entry.duration_seconds) }),
        ),
      ),
    ),
  );
  updatePlaylistCount();
}

async function loadMorePlaylist(button) {
  const current = playlist?.list;
  if (!current?.next) return;
  button.disabled = true;
  button.textContent = 'Loading…';
  try {
    const page = await client.listPlaylist(playlist.url, { start: current.next });
    if (playlist?.list !== current) return; // the link changed meanwhile
    const known = new Set(current.entries.map((e) => e.video_id));
    current.entries.push(...page.entries.filter((e) => !known.has(e.video_id)));
    current.next = page.next;
    current.total = page.total ?? current.total;
    current.unavailable = (current.unavailable || 0) + (page.unavailable || 0);
    renderPlaylist(current, true);
  } catch (err) {
    toast(err.message, true);
  } finally {
    button.disabled = false;
    button.textContent = 'Load next 50';
  }
}

function exitPlaylist() {
  playlistCtl?.abort();
  playlistCtl = null;
  playlist = null;
  $('#playlist').hidden = true;
  updatePlaylistCount();
  updateClipVisibility();
}

function oneLine(text) {
  return text.trim().split(/\s+/).join(' ');
}

async function submitPlaylist() {
  const ids = playlistIds();
  const preset = selectedChoice().replace(/^preset:/, '');
  const folder = !playlist.batch && $('#playlist-folder').checked ? playlist.list.title || 'Playlist' : undefined;
  const button = $('#submit');
  button.disabled = true;
  button.textContent = 'Adding…';
  try {
    const result = await client.createPlaylistJobs({ video_ids: ids, preset, folder });
    if (!client.isFixture) jobs = await client.listJobs();
    const added = result.jobs.length;
    const skipped = result.skipped ? ` ${result.skipped} already in the queue.` : '';
    const into = folder && result.jobs[0]?.folder ? ` Saving into the “${result.jobs[0].folder}” folder.` : '';
    toast(`Added ${added} video${added === 1 ? '' : 's'}.${skipped}${into}`);
    $('#url').value = '';
    resetFormats();
    render();
  } catch (err) {
    toast(err.message, true);
    updatePlaylistCount();
  }
}

// ---------- output folder ----------

// settings: { downloads_dir, configured, default_dir, can_pick }. On first run nothing is
// downloaded until a folder is chosen; afterwards "Change…" opens the same folder window.
let settings = null;

function applySettings(next) {
  settings = next;
  const configured = next.configured !== false;
  $('#setup-folder').hidden = configured;
  $('#setup-pick').hidden = !next.can_pick;
  $('#setup-default-path').textContent = next.default_dir || 'the default folder';
  $('#setup-default').hidden = !next.default_dir;
  $('#dir-path').textContent = next.downloads_dir;
  $('#dir-type').hidden = !next.can_pick;
  renderSignIn(next);
  renderPreferences(next);
  $('#output-dir').hidden = !configured && $('#dir-form').hidden;
  if (configured) setSetupError('');
}

// The section stays hidden when the server has no settings route or can't be reached;
// the health banner already reports an unreachable server.
async function loadSettings() {
  try {
    applySettings(await client.getSettings());
  } catch {
    $('#output-dir').hidden = true;
    $('#setup-folder').hidden = true;
  }
}

function needsFolder() {
  return settings && settings.configured === false;
}

function setSetupError(message) {
  $('#setup-error').textContent = message;
  $('#setup-error').hidden = !message;
}

async function withWaiting(button, label, work) {
  const original = button.textContent;
  button.disabled = true;
  button.classList.add('is-waiting');
  button.textContent = label;
  try {
    return await work();
  } finally {
    button.disabled = false;
    button.classList.remove('is-waiting');
    button.textContent = original;
  }
}

async function pickFolder(button) {
  setSetupError('');
  try {
    const result = await withWaiting(button, 'Waiting for folder window…', () => client.pickFolder());
    if (result.cancelled) return;
    applySettings(result);
    setDirEditing(false, false);
    toast(`Downloads will be saved to ${result.downloads_dir}`);
  } catch (err) {
    if (err.code === 'picker_unavailable' || err.code === 'picker_failed') {
      applySettings({ ...settings, can_pick: false });
      setDirEditing(true);
      setDirError(err.message);
    } else if (needsFolder()) {
      setSetupError(err.message);
    } else {
      toast(err.message, true);
    }
  }
}

async function useDefaultFolder(button) {
  setSetupError('');
  try {
    const result = await withWaiting(button, 'Saving…', () => client.useDefaultFolder());
    applySettings(result);
    toast(`Downloads will be saved to ${result.downloads_dir}`);
  } catch (err) {
    setSetupError(err.message);
  }
}

function setDirEditing(editing, focus = true) {
  $('#output-dir').hidden = false;
  $('#dir-view').hidden = editing || needsFolder();
  $('#dir-form').hidden = !editing;
  setDirError('');
  if (editing) {
    $('#dir-input').value = needsFolder() ? '' : settings?.downloads_dir || '';
    if (focus) {
      $('#dir-input').select();
      $('#dir-input').focus();
    }
  } else {
    $('#output-dir').hidden = needsFolder();
    if (focus && !needsFolder()) $('#dir-edit').focus();
  }
}

function setDirError(message) {
  $('#dir-error').textContent = message;
  $('#dir-error').hidden = !message;
  $('#dir-input').setAttribute('aria-invalid', message ? 'true' : 'false');
}

async function onDirSubmit(e) {
  e.preventDefault();
  const value = $('#dir-input').value.trim();
  if (!value) {
    setDirError('Enter a folder path.');
    return;
  }
  if (!needsFolder() && value === settings?.downloads_dir) {
    setDirEditing(false);
    return;
  }
  const save = $('#dir-save');
  save.disabled = true;
  try {
    applySettings(await client.updateSettings({ downloads_dir: value }));
    setDirEditing(false);
    toast('Output folder saved. New downloads will go there.');
  } catch (err) {
    setDirError(err.message);
  } finally {
    save.disabled = false;
  }
}

// Parallel downloads and the default format; both apply right away.
let presetApplied = false;
// Set once the user picks a format or kind, so a late-loading default never overrides it.
let formatTouched = false;

// Language names in English, from the browser where it knows them.
const languageNames = (() => {
  try {
    const names = new Intl.DisplayNames(['en'], { type: 'language' });
    return (code) => names.of(code) || code;
  } catch {
    return (code) => code;
  }
})();

function renderSubtitles(next) {
  const supported = typeof next.subtitles_mode === 'string' && Array.isArray(next.subtitle_languages);
  $('#pref-subs-row').hidden = !supported;
  $('#pref-subs-lang-row').hidden = !supported || next.subtitles_mode === 'off';
  if (!supported) return;
  const lang = $('#pref-subs-lang');
  if (lang.options.length !== next.subtitle_languages.length) {
    lang.replaceChildren(
      ...next.subtitle_languages
        .map((code) => ({ code, name: languageNames(code) }))
        .sort((a, b) => a.name.localeCompare(b.name))
        .map(({ code, name }) => el('option', { value: code, textContent: name })),
    );
  }
  $('#pref-subs').value = next.subtitles_mode;
  lang.value = next.subtitles_lang;
}

async function saveSubtitles(control) {
  const mode = $('#pref-subs').value;
  const lang = $('#pref-subs-lang').value;
  control.disabled = true;
  try {
    applySettings(await client.setSubtitles({ mode, lang }));
    const name = languageNames(lang);
    toast(
      mode === 'off'
        ? 'Videos will download without subtitles.'
        : mode === 'embed'
          ? `${name} subtitles will be added to new video downloads.`
          : `${name} subtitles will be saved next to new video downloads.`,
    );
  } catch (err) {
    applySettings(settings);
    toast(err.message, true);
  } finally {
    control.disabled = false;
  }
}

function speedLabel(kbps) {
  return kbps ? `${kbps / 1000} MB/s` : 'No limit';
}

function renderSpeed(next) {
  const row = $('#pref-speed-row');
  row.hidden = !Array.isArray(next.speed_limits);
  if (row.hidden) return;
  const select = $('#pref-speed');
  if (select.options.length !== next.speed_limits.length) {
    select.replaceChildren(...next.speed_limits.map((k) => el('option', { value: String(k), textContent: speedLabel(k) })));
  }
  select.value = String(next.speed_limit_kbps);
}

function renderPreferences(next) {
  $('#pref-autoupdate-row').hidden = typeof next.auto_update_ytdlp !== 'boolean';
  $('#pref-autoupdate').checked = Boolean(next.auto_update_ytdlp);
  renderSubtitles(next);
  renderSpeed(next);
  // Only sent where YTGrab can start at sign-in (Windows).
  $('#pref-login-row').hidden = typeof next.start_at_login !== 'boolean';
  if (!$('#pref-login-row').hidden) {
    $('#pref-login').checked = next.start_at_login;
    $('#pref-login-label').textContent = next.start_at_login_label || 'Start with Windows';
  }
  const parallel = $('#pref-parallel');
  $('#pref-parallel-row').hidden = !next.max_downloads;
  $('#pref-preset-row').hidden = !next.default_preset;
  if (next.max_downloads) {
    const limit = next.max_downloads_limit || 4;
    if (parallel.options.length !== limit) {
      parallel.replaceChildren(
        ...Array.from({ length: limit }, (_, i) =>
          el('option', { value: String(i + 1), textContent: i === 0 ? '1 at a time' : `${i + 1} at a time` }),
        ),
      );
    }
    parallel.value = String(next.max_downloads);
  }
  if (next.default_preset) {
    $('#pref-preset').value = next.default_preset;
    // Select the default once on load, unless a link is already being worked on.
    if (!presetApplied && !formatTouched && !$('#url').value) {
      presetApplied = true;
      showPresets(`preset:${next.default_preset}`);
    }
  }
}

async function onStartAtLoginChange(e) {
  const box = e.currentTarget;
  box.disabled = true;
  try {
    applySettings(await client.setStartAtLogin(box.checked));
    toast(box.checked ? 'YTGrab will start in the tray when you sign in.' : "YTGrab won't start when you sign in.");
  } catch (err) {
    box.checked = !box.checked;
    toast(err.message, true);
  } finally {
    box.disabled = false;
  }
}

async function savePreference(select, body, message) {
  select.disabled = true;
  try {
    applySettings(await client.setPreferences(body));
    toast(message);
  } catch (err) {
    applySettings(settings);
    toast(err.message, true);
  } finally {
    select.disabled = false;
  }
}

const BROWSER_NAMES = {
  firefox: 'Firefox', chrome: 'Chrome', edge: 'Edge', brave: 'Brave',
  chromium: 'Chromium', opera: 'Opera', vivaldi: 'Vivaldi', safari: 'Safari',
};

// Browser sign-in is an opt-in, advanced setting; the server accepts only listed browsers.
function renderSignIn(next) {
  const box = $('#signin');
  box.hidden = !Array.isArray(next.cookie_browsers);
  if (box.hidden) return;
  const select = $('#signin-browser');
  if (select.options.length !== next.cookie_browsers.length + 1) {
    select.replaceChildren(
      el('option', { value: '', textContent: 'Off (recommended)' }),
      ...next.cookie_browsers.map((b) => el('option', { value: b, textContent: `Use ${BROWSER_NAMES[b] || b}` })),
    );
  }
  select.value = next.cookies_browser || '';
}

async function onSignInChange(e) {
  const select = e.currentTarget;
  const previous = settings?.cookies_browser || '';
  select.disabled = true;
  try {
    applySettings(await client.setCookiesBrowser(select.value));
    toast(select.value ? `Downloads will use your ${BROWSER_NAMES[select.value] || select.value} YouTube sign-in.` : 'YouTube sign-in turned off.');
  } catch (err) {
    select.value = previous;
    toast(err.message, true);
  } finally {
    select.disabled = false;
  }
}

// Called before queueing anything: points the user at the folder choice on first run.
function requireFolder() {
  if (!needsFolder()) return true;
  setSetupError('Choose a download folder first.');
  $('#setup-folder').scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  ($('#setup-pick').hidden ? $('#setup-default') : $('#setup-pick')).focus();
  return false;
}

// ---------- YTGrab updates ----------

let versionInfo = null;

async function loadVersion() {
  try {
    versionInfo = await client.version();
  } catch {
    return; // older server or offline: no notice
  }
  renderUpdate();
}

function renderUpdate() {
  const info = versionInfo;
  let dismissed = '';
  try {
    dismissed = localStorage.getItem(UPDATE_DISMISSED_KEY) || '';
  } catch {
    // Storage unavailable: the notice can't be dismissed for good.
  }
  const show = Boolean(info?.update_available && info.latest && dismissed !== info.latest);
  $('#update-banner').hidden = !show;
  if (!show) return;
  $('#update-title').textContent = `YTGrab ${info.latest} is available (you have ${info.version}).`;
  const command = info.update_command || '';
  $('#update-how').textContent = command
    ? 'To update, quit YTGrab, run this, then start it again:'
    : 'Download it from the release page and replace the files in your YTGrab folder.';
  $('#update-command').hidden = !command;
  $('#update-command').textContent = command;
  $('#update-copy').hidden = !command;
  $('#update-notes').href = info.release_url;
}

async function copyUpdateCommand() {
  try {
    await navigator.clipboard.writeText(versionInfo.update_command);
    toast('Command copied.');
  } catch {
    toast('Copy the command from the notice.', true);
  }
}

function dismissUpdate() {
  try {
    localStorage.setItem(UPDATE_DISMISSED_KEY, versionInfo.latest);
  } catch {
    // Storage unavailable: hide it for this page only.
  }
  $('#update-banner').hidden = true;
}

function init() {
  $('#fixture-note').hidden = !client.isFixture;
  $('#dir-edit').addEventListener('click', (e) =>
    settings?.can_pick ? pickFolder(e.currentTarget) : setDirEditing(true),
  );
  $('#dir-type').addEventListener('click', () => setDirEditing(true));
  $('#dir-cancel').addEventListener('click', () => setDirEditing(false));
  $('#setup-pick').addEventListener('click', (e) => pickFolder(e.currentTarget));
  $('#setup-default').addEventListener('click', (e) => useDefaultFolder(e.currentTarget));
  $('#setup-type').addEventListener('click', () => setDirEditing(true));
  $('#dir-form').addEventListener('submit', onDirSubmit);
  $('#dir-form').addEventListener('keydown', (e) => {
    if (e.key === 'Escape') setDirEditing(false);
  });
  $('#add-form').addEventListener('submit', onSubmit);
  // A text field drops line breaks, which would run several pasted links together.
  $('#url').addEventListener('paste', (e) => {
    const text = e.clipboardData?.getData('text') || '';
    if (!/\s/.test(text.trim())) return;
    e.preventDefault();
    const input = e.currentTarget;
    input.setRangeText(oneLine(text), input.selectionStart, input.selectionEnd, 'end');
    input.dispatchEvent(new InputEvent('input', { inputType: 'insertFromPaste' }));
  });
  $('#url').addEventListener('input', () => updateClipVisibility());
  for (const id of ['#clip-start', '#clip-end']) $(id).addEventListener('input', () => setClipError(''));
  $('#url').addEventListener('input', (e) => {
    if ($('#url').getAttribute('aria-invalid') === 'true') setUrlError('');
    scheduleInspect(e.inputType === 'insertFromPaste');
  });
  $('#pause-resume').addEventListener('click', (e) => resumeNow(e.currentTarget));
  $('#clear-history').addEventListener('click', clearHistory);
  $('#retry-failed').addEventListener('click', (e) => retryFailed(e.currentTarget));
  loadNotifyPreference();
  renderNotifyToggle();
  $('#notify-toggle').addEventListener('click', toggleNotifications);
  for (const b of document.querySelectorAll('.nav-item')) b.addEventListener('click', () => showView(b.dataset.view));
  $('#now').addEventListener('click', () => showView('download'));
  window.addEventListener('hashchange', () => showView(location.hash.slice(1)));
  showView(location.hash.slice(1));
  for (const tab of document.querySelectorAll('.kind-tab'))
    tab.addEventListener('click', () => {
      formatTouched = true;
      setKind(tab.dataset.kind, { pickFirst: true });
    });
  $('#history-search').addEventListener('input', (e) => {
    historyQuery = e.currentTarget.value.trim().toLowerCase();
    render();
  });
  setupPasteAndDrop();
  setupSendBookmark();
  $('#signin-browser').addEventListener('change', onSignInChange);
  $('#pref-login').addEventListener('change', onStartAtLoginChange);
  $('#pref-speed').addEventListener('change', (e) => {
    const kbps = Number(e.currentTarget.value);
    savePreference(
      e.currentTarget,
      { speed_limit_kbps: kbps },
      kbps ? `New downloads will each use at most ${speedLabel(kbps)}.` : 'New downloads will run at full speed.',
    );
  });
  $('#pref-autoupdate').addEventListener('change', (e) => {
    const on = e.currentTarget.checked;
    savePreference(
      e.currentTarget,
      { auto_update_ytdlp: on },
      on ? 'YTGrab will keep yt-dlp up to date.' : "yt-dlp won't be updated automatically. Update it from the tools status when downloads fail.",
    );
  });
  $('#pref-subs').addEventListener('change', (e) => saveSubtitles(e.currentTarget));
  $('#pref-subs-lang').addEventListener('change', (e) => saveSubtitles(e.currentTarget));
  $('#pref-parallel').addEventListener('change', (e) => {
    const n = Number(e.currentTarget.value);
    savePreference(e.currentTarget, { max_downloads: n }, n === 1 ? 'Downloads will run one at a time.' : `Up to ${n} downloads will run at once.`);
  });
  $('#pref-preset').addEventListener('change', (e) => {
    const label = e.currentTarget.selectedOptions[0].textContent;
    savePreference(e.currentTarget, { default_preset: e.currentTarget.value }, `New links will start with ${label}.`);
  });
  $('#playlist-items').addEventListener('change', updatePlaylistCount);
  $('#playlist-folder').addEventListener('change', (e) => {
    try {
      localStorage.setItem(PLAYLIST_FOLDER_KEY, e.currentTarget.checked ? '1' : '0');
    } catch {
      // Storage unavailable: the choice lasts for this page only.
    }
  });
  $('#playlist-more').addEventListener('click', (e) => loadMorePlaylist(e.currentTarget));
  $('#playlist-all').addEventListener('change', (e) => {
    for (const box of document.querySelectorAll('#playlist-items input')) box.checked = e.target.checked;
    updatePlaylistCount();
  });
  $('#use-presets').addEventListener('click', () => {
    const grouped = cachedInspection(inspectedUrl);
    showPresets();
    const back = el('button', {
      type: 'button',
      className: 'btn-link',
      textContent: 'Show available formats',
    });
    back.addEventListener('click', () => showFormats(grouped, selectedChoice()));
    setInspectStatus(
      el('strong', { className: 'inspect-title', textContent: grouped?.title || 'Video' }),
      back,
    );
  });

  const paste = $('#paste');
  if (navigator.clipboard?.readText) {
    paste.hidden = false;
    paste.addEventListener('click', async () => {
      try {
        $('#url').value = oneLine(await navigator.clipboard.readText());
        setUrlError('');
        scheduleInspect(true);
        $('#url').focus();
      } catch {
        toast('Clipboard access was blocked. Paste with Ctrl+V instead.', true);
      }
    });
  }

  $('#health').addEventListener('click', () => setHealthOpen($('#health-detail').hidden));

  document.querySelector('.filters').addEventListener('click', (e) => {
    const chip = e.target.closest('.chip');
    if (!chip) return;
    historyFilter = chip.dataset.filter;
    for (const c of document.querySelectorAll('.chip'))
      c.setAttribute('aria-pressed', String(c === chip));
    render();
  });

  $('#main').addEventListener('click', (e) => {
    const b = e.target.closest('button[data-action]');
    if (b) runAction(b.closest('.job').dataset.id, b.dataset.action);
  });

  client.subscribe((next) => {
    jobs = next;
    render();
  });

  $('#update-copy').addEventListener('click', copyUpdateCommand);
  $('#update-dismiss').addEventListener('click', dismissUpdate);

  loadHealth();
  loadSettings();
  loadJobs();
  loadVersion();
  takeLinkFromAddress();
  setInterval(loadVersion, 6 * 60 * 60 * 1000); // the server checks GitHub at most daily
}

init();
