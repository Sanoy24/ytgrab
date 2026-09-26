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
  if (!hasVideo) {
    if (u.searchParams.has('list')) {
      return {
        error:
          "Playlist links aren't supported yet. Open one video from the playlist and copy its link.",
      };
    }
    return {
      error:
        "This link doesn't point to a single video. Open the video and copy the link from the address bar.",
    };
  }
  const note = u.searchParams.has('list')
    ? 'Only this video will be downloaded, not the whole playlist.'
    : '';
  return { url: u.toString(), note };
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

function renderActions(box, job) {
  const want = [];
  if (isActive(job.state)) want.push(['cancel', 'Cancel', 'btn-danger']);
  if (job.state === 'failed' || job.state === 'cancelled') want.push(['retry', 'Retry', '']);
  if (job.state === 'completed' && job.output_path) want.push(['copy', 'Copy path', '']);

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

function render() {
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
    ul.append(li);
  }
  return ul;
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
    pill.dataset.status = ready ? 'ok' : 'error';
    label.textContent = ready
      ? 'Tools ready'
      : `${missingRequired.length || 'Some'} tool${missingRequired.length === 1 ? '' : 's'} missing`;

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
  const input = $('#url');
  const result = validateUrl(input.value);
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
    if (inspectedUrl) resetFormats();
    return;
  }
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
  setInspectStatus(
    el('strong', { className: 'inspect-title', textContent: grouped.title || 'Video' }),
    el('small', { textContent: meta }),
  );
}

// ---------- output folder ----------

let downloadsDir = '';

// The section stays hidden when the server has no settings route or can't be reached;
// the health banner already reports an unreachable server.
async function loadSettings() {
  try {
    const s = await client.getSettings();
    downloadsDir = s.downloads_dir;
    $('#dir-path').textContent = downloadsDir;
    $('#output-dir').hidden = false;
  } catch {
    $('#output-dir').hidden = true;
  }
}

function setDirEditing(editing) {
  $('#dir-view').hidden = editing;
  $('#dir-form').hidden = !editing;
  setDirError('');
  if (editing) {
    $('#dir-input').value = downloadsDir;
    $('#dir-input').select();
    $('#dir-input').focus();
  } else {
    $('#dir-edit').focus();
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
  if (value === downloadsDir) {
    setDirEditing(false);
    return;
  }
  const save = $('#dir-save');
  save.disabled = true;
  try {
    const s = await client.updateSettings({ downloads_dir: value });
    downloadsDir = s.downloads_dir;
    $('#dir-path').textContent = downloadsDir;
    setDirEditing(false);
    toast('Output folder saved. New downloads will go there.');
  } catch (err) {
    setDirError(err.message);
  } finally {
    save.disabled = false;
  }
}

function init() {
  $('#fixture-note').hidden = !client.isFixture;
  $('#dir-edit').addEventListener('click', () => setDirEditing(true));
  $('#dir-cancel').addEventListener('click', () => setDirEditing(false));
  $('#dir-form').addEventListener('submit', onDirSubmit);
  $('#dir-form').addEventListener('keydown', (e) => {
    if (e.key === 'Escape') setDirEditing(false);
  });
  $('#add-form').addEventListener('submit', onSubmit);
  $('#url').addEventListener('input', (e) => {
    if ($('#url').getAttribute('aria-invalid') === 'true') setUrlError('');
    scheduleInspect(e.inputType === 'insertFromPaste');
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
