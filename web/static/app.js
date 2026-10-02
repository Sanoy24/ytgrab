import { createClient, isActive } from './api.js';
import { formatDuration, groupFormats, matchPreset } from './formats.js';

const client = createClient();
const $ = (sel) => document.querySelector(sel);

const PRESET_LABELS = {
  'video-best': 'Video · best',
  'video-1080': 'Video · 1080p',
  'video-720': 'Video · 720p',
  'audio-m4a': 'Audio · M4A',
  'audio-mp3': 'Audio · MP3',
};

const STATE_LABELS = {
  queued: 'Queued',
  inspecting: 'Checking',
  downloading: 'Downloading',
  processing: 'Processing',
  completed: 'Done',
  failed: 'Failed',
  cancelled: 'Cancelled',
};

// Extra guidance appended to server error messages; only where the server's text lacks it.
const ERROR_HINTS = {
  video_unavailable: 'Check that the video is public and the link is correct.',
};

const YT_HOSTS = new Set([
  'youtube.com',
  'www.youtube.com',
  'm.youtube.com',
  'music.youtube.com',
  'youtu.be',
]);

let jobs = [];
let historyFilter = 'all';
const pending = new Set(); // job ids with an action in flight

// ---------- URL validation ----------

// Returns { url } or { error }; `note` is an optional non-blocking remark.
export function validateUrl(raw) {
  let text = raw.trim();
  if (!text) return { error: 'Paste a video link first.' };
  if (/\s/.test(text))
    return { error: "That doesn't look like a link. It should start with https://" };
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(text)) text = `https://${text}`;
  let u;
  try {
    u = new URL(text);
  } catch {
    return { error: "That doesn't look like a link. It should start with https://" };
  }
  if (u.protocol !== 'https:' && u.protocol !== 'http:')
    return { error: 'Only http and https links are supported.' };
  if (!YT_HOSTS.has(u.hostname.toLowerCase()))
    return { error: 'Only YouTube links are supported right now.' };

  const host = u.hostname.toLowerCase();
  const hasVideo =
    (host === 'youtu.be' && u.pathname.length > 1) ||
    u.searchParams.has('v') ||
    /^\/(shorts|live|embed)\/[^/]+/.test(u.pathname);
  const listId = u.searchParams.get('list') || '';
  const isMix = listId.startsWith('RD');
  const playlistUrl = /^[A-Za-z0-9_-]{10,64}$/.test(listId) && !isMix ? u.toString() : '';
  if (!hasVideo) {
    if (playlistUrl) return { playlistUrl };
    if (isMix) {
      return {
        error: "YouTube Mixes can't be downloaded as a playlist. Open one video and copy its link.",
      };
    }
    return {
      error:
        "This link doesn't point to a video or playlist. Open the video and copy the link from the address bar.",
    };
  }
  const note = listId ? 'Only this video will be downloaded, not the whole playlist.' : '';
  return { url: u.toString(), note, playlistUrl };
}

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

function jobTitle(job) {
  if (job.title) return job.title;
  return job.video_id ? `Video ${job.video_id}` : job.url;
}

// ---------- rendering ----------

const template = $('#job-template');
const nodes = new Map(); // job id -> element, reused so focus survives updates

function renderJob(job) {
  let el = nodes.get(job.id);
  if (!el) {
    el = template.content.firstElementChild.cloneNode(true);
    el.dataset.id = job.id;
    nodes.set(job.id, el);
  }
  const title = el.querySelector('.job-title');
  title.textContent = jobTitle(job);
  title.title = job.url;

  const badge = el.querySelector('.badge');
  badge.dataset.state = job.state;
  badge.textContent = STATE_LABELS[job.state] || job.state;

  const format = PRESET_LABELS[job.preset] || job.format?.label || job.preset || 'Custom format';
  const meta = [format, formatWhen(job.updated_at)];
  if (job.attempt > 1) meta.push(`attempt ${job.attempt}`);
  el.querySelector('.job-meta').textContent = meta.join(' · ');

  renderProgress(el, job);

  const path = el.querySelector('.job-path');
  path.hidden = !(job.state === 'completed' && job.output_path);
  path.textContent = job.output_path || '';

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
  box.hidden = !showStates.includes(job.state);
  if (box.hidden) return;

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

function renderActions(box, job) {
  const want = [];
  if (isActive(job.state)) want.push(['cancel', 'Cancel', 'btn-danger']);
  if (job.state === 'failed' || job.state === 'cancelled') want.push(['retry', 'Retry', ''], ['remove', 'Remove', '']);
  if (job.state === 'completed' && confirmingRemove.has(job.id)) {
    want.push(['remove-list', 'Remove from list', ''], ['remove-file', 'Delete file too', 'btn-danger'], ['remove-keep', 'Keep', '']);
  } else if (job.state === 'completed') {
    if (job.output_path) want.push(['reveal', 'Show in folder', ''], ['copy', 'Copy path', '']);
    want.push(['remove', 'Remove', '']);
  }

  const key = want.map((w) => w[0]).join(',');
  if (box.dataset.key !== key) {
    box.dataset.key = key;
    box.replaceChildren(
      ...want.map(([action, label, cls]) => {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = `btn btn-small ${cls}`.trim();
        b.dataset.action = action;
        b.textContent = label;
        b.setAttribute('aria-label', `${label}: ${jobTitle(job)}`);
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

const RUN_ORDER = { processing: 0, downloading: 1, inspecting: 2, queued: 3 };

// ---------- finish notifications and tab title ----------

// Notifications are opt-in (the browser asks for permission) and only fire while the
// YTGrab tab is in the background. The choice is remembered in this browser.
const NOTIFY_KEY = 'ytgrab.notify';
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
      (a, b) => RUN_ORDER[a.state] - RUN_ORDER[b.state] || a.created_at.localeCompare(b.created_at),
    );
  const history = jobs
    .filter((j) => !isActive(j.state))
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  const shown = history.filter((j) =>
    historyFilter === 'all'
      ? true
      : historyFilter === 'completed'
        ? j.state === 'completed'
        : j.state !== 'completed',
  );

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
  if (!history.length)
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
  try {
    await (action === 'cancel' ? client.cancelJob(id) : client.retryJob(id));
    if (!client.isFixture) jobs = await client.listJobs();
    toast(action === 'cancel' ? 'Download cancelled.' : 'Download queued again.');
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

async function onSubmit(e) {
  e.preventDefault();
  if (!requireFolder()) return;
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
  const [kind, value] = String(new FormData(e.target).get('choice')).split(':');
  const body = kind === 'preset' ? { preset: value } : { format: { kind, id: value } };
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
    toast(result.note ? `Added. ${result.note}` : 'Added to the queue.');
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
  const want = checkedValue?.startsWith('preset:') ? checkedValue : 'preset:video-best';
  $('#preset-choices').querySelector(`input[value="${want}"]`).checked = true;
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
    const grouped = { ...groupFormats(info), title: info.title, duration: info.duration_seconds };
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

const MP3_CHOICE = {
  kind: 'preset',
  id: 'audio-mp3',
  label: 'MP3 · converted',
  detail: 'Re-encoded with ffmpeg; uses more CPU',
};

function showFormats(grouped, previous) {
  const { video, audio } = grouped;
  if (!video.length && !audio.length) {
    setInspectStatus('No downloadable formats were listed for this video. Using standard presets.');
    return;
  }
  // Carry a choice made before the list loaded over to the closest real format.
  const all = [...video, ...audio, MP3_CHOICE].map((c) => `${c.kind}:${c.id}`);
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
    ...[...audio, MP3_CHOICE].map((c) => formatRow(c, `${c.kind}:${c.id}` === checked)),
  );

  for (const r of $('#preset-choices').querySelectorAll('input')) r.disabled = true;
  $('#preset-choices').hidden = true;
  $('#format-choices').hidden = false;

  const meta = [
    formatDuration(grouped.duration),
    `${video.length} video · ${audio.length} audio options`,
  ]
    .filter(Boolean)
    .join(' · ');
  const parts = [
    el('strong', { className: 'inspect-title', textContent: grouped.title || 'Video' }),
    el('small', { textContent: meta }),
  ];
  const { playlistUrl } = validateUrl($('#url').value);
  if (playlistUrl) {
    const whole = el('button', { type: 'button', className: 'btn-link', textContent: 'Download the whole playlist instead' });
    whole.addEventListener('click', () => enterPlaylist(playlistUrl));
    parts.push(el('br'), whole);
  }
  setInspectStatus(...parts);
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
  button.textContent = n === 1 ? 'Add 1 video' : `Add ${n} videos`;
  button.disabled = n === 0;
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
  $('#playlist-title').replaceChildren(el('span', { className: 'spinner' }), 'Reading playlist…');
  $('#playlist-meta').textContent = '';
  $('#playlist-items').replaceChildren();
  $('#playlist-note').hidden = true;
  $('#playlist-all').parentElement.hidden = true;
  $('#submit').disabled = true;
  try {
    const list = await client.listPlaylist(url, { signal: ctl.signal });
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

function renderPlaylist(list) {
  $('#playlist-title').textContent = list.title || 'Playlist';
  const n = list.entries.length;
  const shown =
    list.truncated && list.total ? `first ${n} of ${list.total} videos` : `${n} video${n === 1 ? '' : 's'}`;
  $('#playlist-meta').textContent = shown.charAt(0).toUpperCase() + shown.slice(1);
  const notes = [];
  if (list.truncated) notes.push(`Up to 50 videos can be added at once.`);
  if (list.unavailable) {
    notes.push(`${list.unavailable} private or deleted video${list.unavailable === 1 ? ' is' : 's are'} skipped.`);
  }
  $('#playlist-note').textContent = notes.join(' ');
  $('#playlist-note').hidden = !notes.length;
  $('#playlist-all').parentElement.hidden = false;
  $('#playlist-items').replaceChildren(
    ...list.entries.map((entry) =>
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

function exitPlaylist() {
  playlistCtl?.abort();
  playlistCtl = null;
  playlist = null;
  $('#playlist').hidden = true;
  updatePlaylistCount();
}

async function submitPlaylist() {
  const ids = playlistIds();
  const preset = selectedChoice().replace(/^preset:/, '');
  const button = $('#submit');
  button.disabled = true;
  button.textContent = 'Adding…';
  try {
    const result = await client.createPlaylistJobs({ video_ids: ids, preset });
    if (!client.isFixture) jobs = await client.listJobs();
    const added = result.jobs.length;
    const skipped = result.skipped ? ` ${result.skipped} already in the queue.` : '';
    toast(`Added ${added} video${added === 1 ? '' : 's'}.${skipped}`);
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
  $('#url').addEventListener('input', (e) => {
    if ($('#url').getAttribute('aria-invalid') === 'true') setUrlError('');
    scheduleInspect(e.inputType === 'insertFromPaste');
  });
  $('#pause-resume').addEventListener('click', (e) => resumeNow(e.currentTarget));
  $('#clear-history').addEventListener('click', clearHistory);
  loadNotifyPreference();
  renderNotifyToggle();
  $('#notify-toggle').addEventListener('click', toggleNotifications);
  $('#signin-browser').addEventListener('change', onSignInChange);
  $('#playlist-items').addEventListener('change', updatePlaylistCount);
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
        $('#url').value = (await navigator.clipboard.readText()).trim();
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

  document.querySelector('.layout').addEventListener('click', (e) => {
    const b = e.target.closest('button[data-action]');
    if (b) runAction(b.closest('.job').dataset.id, b.dataset.action);
  });

  client.subscribe((next) => {
    jobs = next;
    render();
  });

  loadHealth();
  loadSettings();
  loadJobs();
}

init();
