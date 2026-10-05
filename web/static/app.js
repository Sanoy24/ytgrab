import { createClient, isActive } from './api.js';
import { formatDuration, groupFormats, matchPreset, readSection } from './formats.js';
import { looksLikeSearch, parseLinks, validateUrl, videoIdOf } from './links.js';
import { clearSearch, initSearch, runSearch } from './search.js';
import { $, el, formatBytes, formatEta, formatWhen, ICONS, setThumbnail, skeleton, stateBlock, toast } from './ui.js';
import { initWatching, loadWatches, onWatchAction, onWatchSubmit } from './watching.js';

const client = createClient();

const PRESET_LABELS = {
  'video-best': 'Video · best',
  'video-1080': 'Video · 1080p',
  'video-720': 'Video · 720p',
  'video-480': 'Video · 480p',
  'video-360': 'Video · 360p',
  'audio-m4a': 'Audio · M4A',
  'audio-mp3': 'Audio · MP3',
  'audio-opus': 'Audio · Opus',
  'audio-flac': 'Audio · FLAC',
  'audio-wav': 'Audio · WAV',
  'audio-small': 'Audio · small',
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
// Finished downloads' files, from the server: Map(jobId -> { bytes, missing }).
let libraryFiles = new Map();
let historySort = 'newest';
const HISTORY_SORT_KEY = 'ytgrab.historySort';
let historyQuery = ''; // library search, lower case
const pending = new Set(); // job ids with an action in flight

// ---------- URL validation ----------

// ---------- formatting ----------

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
    JSON.stringify(libraryFiles.get(job.id) || null),
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
  setThumbnail(el.querySelector('.job-thumb img'), job.site ? job.thumbnail : job.video_id);

  const badge = el.querySelector('.badge');
  badge.dataset.state = job.state;
  badge.textContent = STATE_LABELS[job.state] || job.state;

  const format = PRESET_LABELS[job.preset] || job.format?.label || job.preset || 'Custom format';
  const when = formatWhen(job.updated_at);
  const meta = [format, when];
  if (SITE_NAMES[job.site]) meta.unshift(SITE_NAMES[job.site]);
  if (job.section) meta.splice(1, 0, `${formatDuration(job.section.start)}–${formatDuration(job.section.end)}`);
  if (job.split_chapters) meta.splice(1, 0, 'split into chapters');
  if (job.attempt > 1) meta.push(`attempt ${job.attempt}`);
  const file = job.state === 'completed' ? libraryFiles.get(job.id) : null;
  if (file && !file.missing) meta.splice(meta.indexOf(when), 0, formatBytes(file.bytes)); // before the time
  const metaEl = el.querySelector('.job-meta');
  metaEl.textContent = meta.join(' · ');
  if (file?.missing) {
    const note = document.createElement('span'); // `el` is this row here, not the helper
    note.className = 'job-missing';
    note.textContent = 'file moved or deleted';
    metaEl.append(' · ', note);
  }

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
  if (job.state === 'failed') want.push(['report', 'Copy report', '']);
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
  const files = jobs.filter((j) => j.state === 'completed' && j.output_path).length;
  const deleteFiles = await askClear(finished, files);
  if (deleteFiles === null) return;
  try {
    const result = await client.clearHistory(deleteFiles);
    if (!client.isFixture) jobs = await client.listJobs();
    else jobs = jobs.filter((j) => isActive(j.state));
    render();
    toast(clearMessage(result, deleteFiles), deleteFiles && result.kept > 0);
  } catch (err) {
    toast(err.message, true);
  }
}

// Asks how to clear: resolves with false (keep files), true (delete them too), or null.
function askClear(finished, files) {
  const dialog = $('#clear-dialog');
  const box = $('#clear-files');
  const confirmButton = $('#clear-confirm');
  $('#clear-text').textContent = `${finished === 1 ? 'This removes 1 download' : `This removes all ${finished} finished, failed, and cancelled downloads`} from the Library. Downloads in the queue stay.`;
  $('#clear-files-row').hidden = !files;
  $('#clear-files-label').textContent = `Also delete ${files === 1 ? 'the downloaded file' : `the ${files} downloaded files`} from this computer`;
  box.checked = false;
  const sync = () => {
    confirmButton.textContent = box.checked ? `Clear and delete ${files === 1 ? 'file' : 'files'}` : 'Clear list';
    confirmButton.classList.toggle('btn-primary', !box.checked);
    confirmButton.classList.toggle('btn-destructive', box.checked);
  };
  box.onchange = sync;
  sync();
  dialog.returnValue = '';
  dialog.showModal();
  return new Promise((resolve) => {
    dialog.addEventListener('close', () => resolve(dialog.returnValue === 'clear' ? box.checked : null), { once: true });
  });
}

function clearMessage({ removed, files_deleted: deleted = 0, kept = 0 }, deleteFiles) {
  const entries = `${removed} entr${removed === 1 ? 'y' : 'ies'}`;
  if (!deleteFiles) return `Removed ${entries}. Your files are kept.`;
  let text = `Removed ${entries} and deleted ${deleted} file${deleted === 1 ? '' : 's'}.`;
  if (kept) text += ` ${kept} file${kept === 1 ? " couldn't" : "s couldn't"} be deleted (open in another program?), so ${kept === 1 ? 'it stays' : 'they stay'} in the list.`;
  return text;
}

// Queues every failed and cancelled download again; each resumes from its partial file.
// Failures a retry can't fix: the video is gone, private, or copy-protected. Each row
// still offers its own Retry, in case the video comes back.
const LASTING_FAILURES = new Set(['video_unavailable', 'drm_protected']);
const retryable = (j) => (j.state === 'failed' && !LASTING_FAILURES.has(j.error?.code)) || j.state === 'cancelled';
const lasting = (j) => j.state === 'failed' && LASTING_FAILURES.has(j.error?.code);

async function retryFailed(button) {
  const failed = jobs.filter(retryable);
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

// Removes the downloads that failed for good (deleted, private, or copy-protected videos)
// from the list. They saved no file, so there is nothing to delete.
async function removeUnavailable(button) {
  const gone = jobs.filter(lasting);
  if (!gone.length) return;
  button.disabled = true;
  let removed = 0;
  for (const job of gone) {
    try {
      await client.deleteJob(job.id);
      removed++;
    } catch {
      // Already removed elsewhere: skip it.
    }
  }
  jobs = client.isFixture ? jobs.filter((j) => !gone.includes(j)) : await client.listJobs();
  button.disabled = false;
  render();
  toast(removed === 1 ? 'Removed 1 unavailable video from the list.' : `Removed ${removed} unavailable videos from the list.`);
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
  refreshLibraryFilesSoon(history);
  renderLibrarySummary(history);
  const shown = history.filter(
    (j) =>
      (historyFilter === 'all' ? true : historyFilter === 'completed' ? j.state === 'completed' : j.state !== 'completed') &&
      (!historyQuery || jobTitle(j).toLowerCase().includes(historyQuery)),
  );
  const failed = history.filter(retryable);
  $('#retry-failed').hidden = !failed.length;
  $('#retry-failed').textContent = `Retry failed (${failed.length})`;
  const gone = history.filter(lasting);
  $('#remove-unavailable').hidden = !gone.length;
  $('#remove-unavailable').textContent = `Remove unavailable (${gone.length})`;
  $('#queue-nav-count').textContent = queue.length || '';
  $('#library-nav-count').textContent = history.length || '';
  // The counts are drawn as badges; give the buttons names that read naturally.
  setNavLabel('download', 'Download', queue.length, 'in the queue');
  setNavLabel('library', 'Library', history.length, history.length === 1 ? 'download' : 'downloads');
  renderNow(queue);

  $('#queue-count').textContent = queue.length ? `(${queue.length})` : '';
  const opens = client.windowOpens ? new Date(client.windowOpens) : null;
  const waiting = opens && queue.some((j) => j.state === 'queued');
  $('#window-note').hidden = !waiting;
  if (waiting) {
    const time = opens.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    $('#window-note').textContent = `Outside your download window: queued downloads start at ${time}. Change the window in Settings.`;
  }
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
  fillList($('#history'), sortHistory(shown), historyEmpty);

  // Drop cached nodes for jobs that no longer exist.
  const ids = new Set(jobs.map((j) => j.id));
  for (const id of nodes.keys()) if (!ids.has(id)) nodes.delete(id);
}

const bytesOf = (job) => libraryFiles.get(job.id)?.bytes || 0;
const SORTS = {
  newest: (a, b) => b.updated_at.localeCompare(a.updated_at),
  oldest: (a, b) => a.updated_at.localeCompare(b.updated_at),
  largest: (a, b) => bytesOf(b) - bytesOf(a) || b.updated_at.localeCompare(a.updated_at),
  title: (a, b) => jobTitle(a).localeCompare(jobTitle(b), undefined, { sensitivity: 'base', numeric: true }),
};

function sortHistory(list) {
  return historySort === 'newest' ? list : [...list].sort(SORTS[historySort] || SORTS.newest);
}

// "12 files · 3.4 GB on disk · 1 moved or deleted", once the sizes are known.
function renderLibrarySummary(history) {
  const summary = $('#library-summary');
  const files = history.map((j) => (j.state === 'completed' ? libraryFiles.get(j.id) : null)).filter(Boolean);
  const present = files.filter((f) => !f.missing);
  const missing = files.length - present.length;
  summary.hidden = !files.length;
  if (!files.length) return;
  const total = present.reduce((sum, f) => sum + f.bytes, 0);
  const parts = [`${present.length} file${present.length === 1 ? '' : 's'}`, `${formatBytes(total)} on disk`];
  if (missing) parts.push(`${missing} moved or deleted`);
  summary.textContent = parts.join(' · ');
}

// The sizes are read again when the set of finished downloads changes.
let libraryFilesKey = null;
let libraryFilesTimer = 0;
function refreshLibraryFilesSoon(history) {
  const key = history.filter((j) => j.state === 'completed').map((j) => `${j.id}:${j.output_path}`).join('|');
  if (key === libraryFilesKey) return;
  libraryFilesKey = key;
  clearTimeout(libraryFilesTimer);
  libraryFilesTimer = setTimeout(refreshLibraryFiles, 400);
}

async function refreshLibraryFiles() {
  try {
    const { files } = await client.libraryFiles();
    libraryFiles = new Map(Object.entries(files || {}));
    render();
  } catch {
    // Sizes are extra: the Library works without them.
  }
}

// ---------- back up and restore ----------

function plural(n, one, many) {
  return `${n} ${n === 1 ? one : many}`;
}

// Restores a backup file the user picked, then reloads what it may have changed.
async function restoreBackup(input) {
  const file = input.files?.[0];
  input.value = ''; // picking the same file again still restores
  if (!file) return;
  const result = $('#backup-result');
  result.textContent = 'Restoring…';
  try {
    const r = await client.restoreBackup(await file.text());
    const parts = [
      `Added ${plural(r.watches_added, 'watch', 'watches')} and ${plural(r.jobs_added, 'Library entry', 'Library entries')}.`,
    ];
    const kept = r.watches_existing + r.jobs_existing;
    if (kept) parts.push(`${plural(kept, 'item was', 'items were')} already here.`);
    if (r.jobs_invalid) parts.push(`${plural(r.jobs_invalid, 'entry', 'entries')} couldn't be read.`);
    if (r.settings_skipped?.length) parts.push(`Kept this computer's ${r.settings_skipped.join(', ')}.`);
    result.textContent = parts.join(' ');
    toast('Backup restored.');
    if (!client.isFixture) {
      jobs = await client.listJobs();
      render();
      loadSettings();
      loadWatches();
    }
  } catch (err) {
    result.textContent = err.message;
    toast(err.message, true);
  }
}

// Saves the list as shown (search, filter, and sort applied) as a CSV file.
function exportHistory() {
  const shown = [...document.querySelectorAll('#history .job')].map((row) => jobs.find((j) => j.id === row.dataset.id)).filter(Boolean);
  if (!shown.length) {
    toast('Nothing to export.');
    return;
  }
  // A leading =, +, -, or @ would make a spreadsheet run the cell as a formula.
  const cell = (value) => {
    let text = String(value ?? '');
    if (/^[=+\-@\t\r]/.test(text)) text = `'${text}`;
    return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
  };
  const rows = [['Title', 'Site', 'Link', 'Format', 'State', 'Size (bytes)', 'Updated', 'File']];
  for (const job of shown) {
    const file = libraryFiles.get(job.id);
    rows.push([
      jobTitle(job),
      SITE_NAMES[job.site] || 'YouTube',
      job.url,
      PRESET_LABELS[job.preset] || job.format?.label || job.preset || '',
      STATE_LABELS[job.state] || job.state,
      file && !file.missing ? file.bytes : '',
      job.updated_at,
      job.state === 'completed' ? job.output_path || '' : '',
    ]);
  }
  const csv = rows.map((r) => r.map(cell).join(',')).join('\r\n');
  // The byte order mark makes Excel read the file as UTF-8.
  const url = URL.createObjectURL(new Blob(['\ufeff', csv, '\r\n'], { type: 'text/csv;charset=utf-8' }));
  const link = el('a', { href: url, download: `ytgrab-library-${new Date().toISOString().slice(0, 10)}.csv` });
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
  toast(`Exported ${shown.length} download${shown.length === 1 ? '' : 's'}.`);
}

function setNavLabel(view, name, count, noun) {
  const button = document.querySelector(`.nav-item[data-view="${view}"]`);
  button.setAttribute('aria-label', count ? `${name}, ${count} ${noun}` : name);
}

// ---------- views ----------

const VIEWS = ['download', 'library', 'watching', 'settings'];
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
  if (name === 'watching') loadWatches();
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
  // A handed-over link YTGrab can't use says why at once, rather than waiting for Add.
  if (!looksLikeSearch(input.value) && !parseLinks(input.value)) {
    const { error } = validateUrl(input.value);
    if (error) setUrlError(error);
  }
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

let healthReport = null;

async function loadHealth() {
  const pill = $('#health');
  const label = $('#health-label');
  const banner = $('#health-detail');
  pill.hidden = false;
  try {
    const h = await client.health();
    healthReport = h;
    renderAbout();
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
  if (action === 'report') {
    await copyReport(diagnosticReport(job), 'Report copied. Paste it into a bug report or message.');
    return;
  }
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
  if (looksLikeSearch($('#url').value)) {
    runSearch($('#url').value.trim());
    return;
  }
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
  const words = looksLikeSearch($('#url').value);
  $('#search-hint').hidden = !words;
  if (!words) clearSearch();
  if (words) {
    if (inspectedUrl || playlist) resetFormats();
    return;
  }
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
  if (playlist?.url === `post:${result.url}`) return;
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
// An X or Instagram post with several videos opens the review list, unless the link names
// one of them.
function showInspection(url, grouped) {
  if (grouped.videos?.length > 1 && !/\/video\/\d|[?&]item=\d/.test(url)) enterPostVideos(url, grouped);
  else showFormats(grouped, selectedChoice());
}

// The post's videos reuse the review list: all ticked, one format for every video.
function enterPostVideos(url, grouped) {
  const key = `post:${url}`;
  if (playlist?.url === key) return;
  setInspectStatus();
  showPresets(selectedChoice());
  const list = {
    title: `This post has ${grouped.videos.length} videos`,
    meta: 'Each is saved as its own file.',
    batch: true,
    entries: grouped.videos.map((v) => ({
      video_id: v.video_id,
      url: v.url,
      title: `Video ${v.index}`,
      duration_seconds: v.duration_seconds,
    })),
  };
  playlist = { url: key, list, batch: true };
  $('#playlist').hidden = false;
  $('#clip').hidden = true;
  renderPlaylist(list);
}

async function inspect(url) {
  inspectCtl?.abort();
  const ctl = (inspectCtl = new AbortController());
  inspectedUrl = url;

  const cached = cachedInspection(url);
  if (cached) {
    showInspection(url, cached);
    return cached;
  }

  setInspectStatus(el('span', { className: 'spinner' }), 'Checking available formats…');
  try {
    const info = await client.inspect(url, { signal: ctl.signal });
    const grouped = {
      ...groupFormats(info),
      title: info.title,
      duration: info.duration_seconds,
      chapters: info.chapters || 0,
      site: info.site || '',
      thumbnail: info.thumbnail || '',
      videos: info.videos || [],
    };
    inspections.set(url, { grouped, at: Date.now() });
    if (ctl.signal.aborted) return null;
    showInspection(url, grouped);
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

// Sites other than YouTube, as job cards name them.
const SITE_NAMES = { x: 'X', reddit: 'Reddit', instagram: 'Instagram', vimeo: 'Vimeo' };

const X_AUDIO = { kind: 'preset', id: 'audio-m4a', label: 'M4A', detail: "The post's audio, no re-encoding" };

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
  // X lists no separate audio streams: offer M4A taken out of the video, like the presets.
  const conversions = grouped.site === 'x' ? [X_AUDIO, ...AUDIO_CONVERSIONS] : AUDIO_CONVERSIONS;
  const all = [...video, ...audio, ...conversions].map((c) => `${c.kind}:${c.id}`);
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
    ...[...audio, ...conversions].map((c) => formatRow(c, `${c.kind}:${c.id}` === checked)),
  );

  for (const r of $('#preset-choices').querySelectorAll('input')) r.disabled = true;
  $('#preset-choices').hidden = true;
  $('#format-choices').hidden = false;
  syncKind();

  renderSplitOption(grouped.chapters);
  const meta = [
    formatDuration(grouped.duration),
    grouped.chapters ? `${grouped.chapters} chapters` : '',
    grouped.site === 'x' ? `From X · ${video.length} video options` : `${video.length} video · ${audio.length} audio options`,
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
  setThumbnail(thumb, grouped.site ? grouped.thumbnail : url ? videoIdOf(url) : '');
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
  setUrlError(videos.length ? '' : 'None of these are links to single YouTube videos or X posts.');
  if (!videos.length) {
    if (playlist) exitPlaylist();
    return;
  }
  const list = {
    title: 'Several links',
    batch: true,
    entries: videos.map((v) => ({ video_id: v.id, url: v.url, title: v.url.replace(/^https:\/\/(www\.)?/, '') })),
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
  $('#playlist-meta').textContent = list.meta || shown;
  $('#playlist-legend').textContent = list.batch ? 'Videos to add' : 'Playlist videos';
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
          el('input', { type: 'checkbox', value: entry.url || entry.video_id, checked: true }),
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
    // Pasted links and a post's videos are sent as links; a playlist's entries as IDs.
    const result = await client.createPlaylistJobs(playlist.batch ? { urls: ids, preset } : { video_ids: ids, preset, folder });
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

const NAME_EXAMPLES = {
  title: 'Me at the zoo [jNQXAC9IVRw] 720p.mp4',
  'channel-title': 'jawed - Me at the zoo [jNQXAC9IVRw] 720p.mp4',
  'date-title': '2005-04-24 Me at the zoo [jNQXAC9IVRw] 720p.mp4',
  'channel-folder': 'jawed / Me at the zoo [jNQXAC9IVRw] 720p.mp4',
};

function renderFileNames(next) {
  $('#pref-names-row').hidden = typeof next.file_names !== 'string';
  if (typeof next.file_names !== 'string') return;
  $('#pref-names').value = next.file_names;
  $('#pref-names-example').textContent = `For example: ${NAME_EXAMPLES[next.file_names] || ''}`;
}

function renderWindow(next) {
  $('#pref-window-row').hidden = typeof next.download_window !== 'string';
  if (typeof next.download_window !== 'string') return;
  const [start, end] = next.download_window ? next.download_window.split('-') : ['', ''];
  $('#pref-window-start').value = start;
  $('#pref-window-end').value = end || '7';
  $('#pref-window-end').hidden = $('#pref-window-and').hidden = !next.download_window;
}

function saveWindow(control) {
  const start = $('#pref-window-start').value;
  let end = $('#pref-window-end').value;
  if (start !== '' && end === start) end = String((Number(start) + 6) % 24);
  const value = start === '' ? '' : `${start}-${end}`;
  const hh = (h) => `${String(h).padStart(2, '0')}:00`;
  savePreference(control, { download_window: value }, value ? `Downloads will start only between ${hh(start)} and ${hh(end)}.` : 'Downloads can start at any time.');
}

function renderPreferences(next) {
  renderFileNames(next);
  $('#pref-normalize-row').hidden = typeof next.normalize_audio !== 'boolean';
  $('#pref-normalize').checked = Boolean(next.normalize_audio);
  renderWindow(next);
  $('#pref-sponsor-row').hidden = typeof next.sponsorblock !== 'string';
  if (typeof next.sponsorblock === 'string') $('#pref-sponsor').value = next.sponsorblock;
  $('#pref-autoupdate-row').hidden = typeof next.auto_update_ytdlp !== 'boolean';
  $('#pref-channel-row').hidden = typeof next.ytdlp_channel !== 'string';
  $('#pref-metadata-row').hidden = typeof next.save_metadata !== 'boolean';
  $('#pref-metadata').checked = Boolean(next.save_metadata);
  if (next.ytdlp_channel) $('#pref-channel').value = next.ytdlp_channel;
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
  const file = next.cookies_file ? [el('option', { value: 'file', textContent: 'Use a cookies.txt file…' })] : [];
  if (select.options.length !== next.cookie_browsers.length + 1 + file.length) {
    select.replaceChildren(
      el('option', { value: '', textContent: 'Off (recommended)' }),
      ...next.cookie_browsers.map((b) => el('option', { value: b, textContent: `Use ${BROWSER_NAMES[b] || b}` })),
      ...file,
    );
  }
  select.value = next.cookies_browser || '';
  renderSignInWarning(select.value);
}

// Chrome and its relatives on Windows encrypt their sign-in data so yt-dlp often can't read
// it (yt-dlp issue 10927); YTGrab then carries on without sign-in.
const CHROMIUM_BROWSERS = new Set(['chrome', 'edge', 'brave', 'chromium', 'opera', 'vivaldi']);
const onWindows = /win/i.test(navigator.userAgentData?.platform || navigator.platform || '');

function renderSignInWarning(choice) {
  const warning = $('#signin-warning');
  warning.hidden = !(onWindows && CHROMIUM_BROWSERS.has(choice));
  warning.textContent = `On Windows, ${BROWSER_NAMES[choice] || 'this browser'} usually locks its sign-in data so yt-dlp can't read it, and downloads then go ahead without signing in. Firefox works, or export a cookies.txt file from your browser and choose "Use a cookies.txt file…".`;
}

// Picking "Use a cookies.txt file…" asks for the file; closing that window puts the
// previous choice back.
function chooseCookieFile(select, previous) {
  const input = $('#signin-file');
  input.value = '';
  const restore = () => {
    select.value = previous;
    renderSignInWarning(previous);
  };
  input.oncancel = restore;
  input.onchange = async () => {
    const chosen = input.files?.[0];
    if (!chosen) return restore();
    select.disabled = true;
    try {
      applySettings(await client.importCookies(await chosen.text()));
      toast('Downloads will sign in with your cookies.txt file. YTGrab keeps it only on this computer.');
    } catch (err) {
      restore();
      toast(err.message, true);
    } finally {
      select.disabled = false;
    }
  };
  input.click();
}

async function onSignInChange(e) {
  const select = e.currentTarget;
  const previous = settings?.cookies_browser || '';
  if (select.value === 'file') {
    chooseCookieFile(select, previous);
    return;
  }
  renderSignInWarning(select.value);
  select.disabled = true;
  try {
    applySettings(await client.setCookiesBrowser(select.value));
    toast(select.value ? `Downloads will use your ${BROWSER_NAMES[select.value] || select.value} sign-in.` : 'Browser sign-in turned off.');
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
let versionCheckedAt = 0;

async function loadVersion() {
  versionCheckedAt = Date.now();
  try {
    versionInfo = await client.version();
  } catch {
    return; // older server or offline: no notice
  }
  renderUpdate();
}

// A tab left open for days asks again when it comes back into view, at most hourly; the
// server then looks for a new release if its last look is over an hour old.
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'visible' && Date.now() - versionCheckedAt > 60 * 60 * 1000) loadVersion();
});

// ---------- settings sections ----------

// Scrolls only the page's own scrolling area to a section: scrollIntoView would also move
// the outer page, pushing the sidebar out of view.
function scrollToSection(section) {
  const main = $('#main');
  const scroller = main.scrollHeight > main.clientHeight && getComputedStyle(main).overflowY !== 'visible' ? main : document.scrollingElement;
  const top = scroller === main ? main.getBoundingClientRect().top : 0;
  // On narrow screens the sidebar and the section chips stay at the top; clear them.
  const sticky = scroller === main ? 16 : ($('.sidebar').offsetHeight || 0) + ($('.settings-nav').offsetHeight || 0) + 12;
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  scroller.scrollTo({ top: section.getBoundingClientRect().top - top + scroller.scrollTop - sticky, behavior: reduceMotion ? 'auto' : 'smooth' });
}

// The section menu scrolls to a section and marks the one being read. It uses buttons,
// not #links, because the address's #part already picks the view.
function setupSettingsNav() {
  const links = [...document.querySelectorAll('.settings-link')];
  const mark = (id) => links.forEach((l) => l.setAttribute('aria-current', String(l.dataset.section === id)));
  for (const link of links) {
    link.addEventListener('click', () => {
      mark(link.dataset.section);
      const section = document.getElementById(link.dataset.section);
      if (section) scrollToSection(section);
    });
  }
  mark(links[0]?.dataset.section);
  if (!('IntersectionObserver' in window)) return;
  const visible = new Map();
  const observer = new IntersectionObserver((entries) => {
    for (const entry of entries) visible.set(entry.target.id, entry.isIntersecting ? entry.intersectionRatio : 0);
    // The first section with any part on screen is the one being read, except at the very
    // bottom, where the last section can't scroll any higher.
    const main = $('#main');
    const scroller = main.scrollHeight > main.clientHeight && getComputedStyle(main).overflowY !== 'visible' ? main : document.scrollingElement;
    const atBottom = scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 4;
    const ids = links.map((l) => l.dataset.section);
    const current = atBottom ? ids.at(-1) : ids.find((id) => visible.get(id) > 0);
    if (current) mark(current);
  }, { rootMargin: '-10% 0px -55% 0px', threshold: [0, 0.01, 1] });
  for (const link of links) {
    const section = document.getElementById(link.dataset.section);
    if (section) observer.observe(section);
  }
}

// ---------- diagnostic report ----------

// Versions, settings that affect downloads, and, for a failed download, what went wrong in
// YTGrab's words and yt-dlp's. Copied for the user to paste into a bug report; it includes
// the link, so it's theirs to share or not.
function diagnosticReport(job) {
  const tool = (name) => healthReport?.dependencies.find((d) => d.name === name);
  const describe = (d) => (d?.available ? `${shortVersion(d.version)}${d.outdated ? ' (update available)' : ''}` : 'not found');
  const platform = navigator.userAgentData?.platform || navigator.platform || 'unknown';
  const lines = [
    'YTGrab diagnostic report',
    `YTGrab: ${versionInfo?.version || 'unknown'} on ${platform}`,
    `yt-dlp: ${describe(tool('yt-dlp'))}`,
    `FFmpeg: ${describe(tool('ffmpeg'))}`,
    `JavaScript runtime: ${describe(tool('js-runtime'))}`,
    `Browser sign-in: ${$('#signin-browser')?.value || 'off'}`,
  ];
  if (job) {
    lines.push(
      '',
      `Link: ${job.url}`,
      `Format: ${PRESET_LABELS[job.preset] || job.format?.label || job.preset || 'custom'}`,
      `Attempt: ${job.attempt}`,
      `Error: ${job.error?.code || 'none'} (${job.error?.message || ''})`,
    );
    if (job.error?.detail) lines.push('yt-dlp said:', job.error.detail);
  }
  return lines.join('\n');
}

async function copyReport(text, done) {
  try {
    await navigator.clipboard.writeText(text);
    toast(done);
  } catch {
    toast("Couldn't copy the report. Your browser blocked the clipboard.", true);
  }
}

// ---------- about ----------

function renderAbout() {
  const info = versionInfo;
  if (info) {
    $('#about-version').textContent = info.version || 'development build';
    const status = $('#about-status');
    status.classList.toggle('is-new', Boolean(info.update_available));
    status.textContent = info.update_available
      ? `${info.latest} is available`
      : info.latest
        ? 'up to date'
        : '';
    $('#app-version').hidden = !info.version;
    $('#app-version').textContent = `YTGrab ${info.version}${info.update_available ? ' · update available' : ''}`;
  }
  const tool = (name) => healthReport?.dependencies.find((d) => d.name === name);
  const describe = (d) => {
    if (!healthReport) return '…';
    if (!d || !d.available) return 'Not found';
    return `${shortVersion(d.version)}${d.outdated ? ' · update available' : ''}`;
  };
  $('#about-ytdlp').textContent = describe(tool('yt-dlp'));
  $('#about-ffmpeg').textContent = describe(tool('ffmpeg'));
  // yt-dlp can use Deno or Node; name the one found.
  const js = tool('js-runtime');
  const jsName = /node(\.exe)?$/i.test(js?.path || '') ? 'Node.js ' : /deno(\.exe)?$/i.test(js?.path || '') ? 'Deno ' : '';
  $('#about-js').textContent = js?.available ? jsName + describe(js) : describe(js);
}

async function checkForUpdates(button) {
  button.disabled = true;
  button.textContent = 'Checking…';
  try {
    versionInfo = await client.version({ refresh: true });
    versionCheckedAt = Date.now();
    renderUpdate();
    toast(versionInfo.update_available ? `YTGrab ${versionInfo.latest} is available.` : versionInfo.latest ? 'YTGrab is up to date.' : "Couldn't reach GitHub to check. Try again later.", !versionInfo.latest);
  } catch (err) {
    toast(err.message, true);
  } finally {
    button.disabled = false;
    button.textContent = 'Check for updates';
  }
}

function renderUpdate() {
  renderAbout();
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
  const command = info.can_update ? '' : info.update_command || '';
  $('#update-how').textContent = info.can_update
    ? 'YTGrab can install it for you: it downloads the update, checks it, and restarts. Your downloads list is kept.'
    : command
      ? 'To update, quit YTGrab, run this, then start it again:'
      : 'Download it from the release page and replace the files in your YTGrab folder.';
  $('#update-now').hidden = !info.can_update;
  $('#update-command').hidden = !command;
  $('#update-command').textContent = command;
  $('#update-copy').hidden = !command;
  $('#update-notes').href = info.release_url;
}

// Installs the update, then waits for the restarted YTGrab to answer with the new version
// and reloads the page from it.
async function installUpdate(button) {
  button.disabled = true;
  button.textContent = 'Updating…';
  $('#update-how').textContent = 'Downloading and checking the update. This takes a moment.';
  let result;
  try {
    result = await client.updateYTGrab();
  } catch (err) {
    toast(err.message, true);
    button.disabled = false;
    button.textContent = 'Update now';
    renderUpdate();
    return;
  }
  $('#update-title').textContent = `Restarting YTGrab ${result.version}…`;
  $('#update-how').textContent = 'The page reloads when it is back.';
  button.hidden = true;
  if (client.isFixture) {
    versionInfo = await client.version();
    renderUpdate();
    toast(`YTGrab ${result.version} is installed.`);
    return;
  }
  const deadline = Date.now() + 90_000;
  while (Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 1000));
    try {
      if ((await client.version()).version === result.version) {
        location.reload();
        return;
      }
    } catch {
      // Still restarting.
    }
  }
  $('#update-how').textContent = "YTGrab hasn't come back yet. Start it again from the Start menu or your terminal, then reload this page.";
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
  initSearch({ client, open: takeLinks });
  initWatching({
    client,
    presetLabels: PRESET_LABELS,
    setNavLabel,
    refreshJobs: async () => {
      if (!client.isFixture) jobs = await client.listJobs();
      render();
    },
  });
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
  $('#remove-unavailable').addEventListener('click', (e) => removeUnavailable(e.currentTarget));
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
  try {
    historySort = localStorage.getItem(HISTORY_SORT_KEY) || 'newest';
  } catch {
    // Storage blocked: the default order is fine.
  }
  if (!SORTS[historySort]) historySort = 'newest';
  $('#history-sort').value = historySort;
  $('#history-sort').addEventListener('change', (e) => {
    historySort = e.currentTarget.value;
    try {
      localStorage.setItem(HISTORY_SORT_KEY, historySort);
    } catch {
      // Remembering the order is only a convenience.
    }
    render();
  });
  $('#export-history').addEventListener('click', exportHistory);
  $('#about-check').addEventListener('click', (e) => checkForUpdates(e.currentTarget));
  $('#about-report').addEventListener('click', () => copyReport(diagnosticReport(null), 'Diagnostic report copied.'));
  setupSettingsNav();
  $('#app-version').addEventListener('click', () => {
    showView('settings');
    scrollToSection($('#about'));
  });
  $('#backup-file').addEventListener('change', (e) => restoreBackup(e.currentTarget));
  if (client.isFixture) for (const id of ['#backup-save', '#archive-save']) $(id).removeAttribute('href'); // no server to save from
  $('#history-search').addEventListener('input', (e) => {
    historyQuery = e.currentTarget.value.trim().toLowerCase();
    render();
  });
  setupPasteAndDrop();
  $('#watch-form').addEventListener('submit', onWatchSubmit);
  $('#watch-url').addEventListener('input', () => ($('#watch-error').hidden = true));
  $('#watch-list').addEventListener('click', (e) => {
    const b = e.target.closest('button[data-watch-action]');
    if (b) onWatchAction(b.closest('.watch').dataset.id, b.dataset.watchAction);
  });
  setInterval(() => currentView === 'watching' && loadWatches(), 60_000);
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
  for (const id of ['#pref-window-start', '#pref-window-end']) $(id).addEventListener('change', (e) => saveWindow(e.currentTarget));
  // Many mobile plans sell cheaper data overnight (Ethio telecom's night bundles run 22:00-08:00).
  $('#pref-window-night').addEventListener('click', () => {
    $('#pref-window-start').value = '22';
    $('#pref-window-end').value = '8';
    saveWindow($('#pref-window-start'));
  });
  $('#pref-normalize').addEventListener('change', (e) => {
    const on = e.currentTarget.checked;
    savePreference(e.currentTarget, { normalize_audio: on }, on ? 'MP3, FLAC, and WAV downloads will be evened out in loudness.' : 'Audio loudness will be left as it is.');
  });
  $('#pref-sponsor').addEventListener('change', (e) => {
    const mode = e.currentTarget.value;
    savePreference(
      e.currentTarget,
      { sponsorblock: mode },
      mode === 'off' ? 'Sponsor segments will be left as they are.' : mode === 'mark' ? 'Sponsor segments will be marked as chapters in new downloads.' : 'Sponsor segments will be cut out of new downloads.',
    );
  });
  $('#pref-names').addEventListener('change', (e) => {
    const label = e.currentTarget.selectedOptions[0].textContent;
    savePreference(e.currentTarget, { file_names: e.currentTarget.value }, `New downloads will be named by ${label.toLowerCase()}.`);
  });
  $('#pref-metadata').addEventListener('change', (e) => {
    const on = e.currentTarget.checked;
    savePreference(e.currentTarget, { save_metadata: on }, on ? 'New downloads will keep their details, description, and thumbnail as files.' : 'New downloads will keep only the video or audio file.');
  });
  $('#pref-channel').addEventListener('change', async (e) => {
    const nightly = e.currentTarget.value === 'nightly';
    await savePreference(
      e.currentTarget,
      { ytdlp_channel: e.currentTarget.value },
      nightly
        ? 'Switching to Nightly yt-dlp builds. The new build installs in the background, between downloads.'
        : 'Switching back to Stable yt-dlp builds. The stable build installs in the background, between downloads.',
    );
    setTimeout(loadHealth, 20000); // show the installed version once it's in
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
  $('#update-now').addEventListener('click', (e) => installUpdate(e.currentTarget));
  $('#update-dismiss').addEventListener('click', dismissUpdate);

  loadHealth();
  loadSettings();
  loadJobs();
  loadVersion();
  loadWatches(); // for the sidebar count
  takeLinkFromAddress();
  setInterval(loadVersion, 6 * 60 * 60 * 1000); // the server checks GitHub at most daily
}

init();
