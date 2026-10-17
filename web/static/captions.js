// Local captions: whisper.cpp makes subtitles on this computer for downloads without any.
// Settings → Audio & video installs it; the Library's "Make captions" queues a download.
import { $, toast } from './ui.js';

let client;
let hooks; // { render, refreshFiles, titleOf }
let status = null; // { supported, installed, installing, progress, error, how_to, size, jobs }
let pollTimer = 0;

export function initCaptions(apiClient, callbacks) {
  client = apiClient;
  hooks = callbacks;
  $('#captions-install')?.addEventListener('click', install);
  $('#pref-auto-captions')?.addEventListener('change', setAutomatic);
  load();
}

export function captionsStatus() {
  return status;
}

async function load() {
  let next;
  try {
    next = await client.captionsStatus();
  } catch {
    return; // an older server, or none: no captions
  }
  const before = status?.jobs || {};
  status = next;
  for (const [id, job] of Object.entries(status.jobs || {})) {
    if (job.state === before[id]?.state) continue;
    if (job.state === 'done' && before[id]) {
      toast(`Captions ready for ${hooks.titleOf(id)}. They're searchable now.`);
      hooks.refreshFiles();
    } else if (job.state === 'failed' && before[id]) {
      toast(`Couldn't make captions: ${job.error}`, true);
    }
  }
  renderSetting();
  hooks.render();
  const busy = status.installing || Object.values(status.jobs || {}).some((j) => j.state === 'queued' || j.state === 'making');
  clearTimeout(pollTimer);
  if (busy) pollTimer = setTimeout(load, 2000);
}

// Called with the settings as they load or change: { auto_captions }.
export function applyCaptionSettings(settings) {
  const box = $('#pref-auto-captions');
  if (box && typeof settings?.auto_captions === 'boolean') box.checked = settings.auto_captions;
}

async function setAutomatic(e) {
  const box = e.currentTarget;
  box.disabled = true;
  try {
    await client.setPreferences({ auto_captions: box.checked });
    toast(box.checked ? 'New downloads without subtitles will get captions by themselves.' : 'Automatic captions are off.');
  } catch (err) {
    box.checked = !box.checked;
    toast(err.message, true);
  } finally {
    box.disabled = false;
  }
}

function renderSetting() {
  const row = $('#captions-row');
  if (!row) return;
  row.hidden = !status.supported;
  $('#captions-auto-row').hidden = !status.installed;
  const button = $('#captions-install');
  const note = $('#captions-state');
  if (status.installed) {
    button.hidden = true;
    note.textContent = 'Installed. Use Make captions on a finished download in the Library.';
  } else if (status.installing) {
    button.hidden = false;
    button.disabled = true;
    button.textContent = `Installing… ${Math.round((status.progress || 0) * 100)}%`;
    note.textContent = '';
  } else {
    button.hidden = false;
    button.disabled = !!status.how_to;
    button.textContent = `Install (${status.size})`;
    note.textContent = status.how_to || status.error || '';
  }
}

async function install(e) {
  e.currentTarget.disabled = true;
  try {
    status = await client.installCaptions();
    renderSetting();
    load();
  } catch (err) {
    toast(err.message, true);
    e.currentTarget.disabled = false;
  }
}

// The Library button for a download: "captions" to make them, "captions-busy" while they
// are on the way, or null when there's nothing to offer.
export function captionAction(job, file) {
  if (!status?.installed || job.state !== 'completed' || !job.output_path || !file || file.missing) return null;
  const state = status.jobs?.[job.id]?.state;
  if (state === 'queued') return ['captions-busy', 'Captions queued', ''];
  if (state === 'making') return ['captions-busy', 'Making captions…', ''];
  if (file.subtitles || state === 'done') return null;
  return ['captions', 'Make captions', ''];
}

export async function requestCaptions(id) {
  try {
    status = await client.requestCaptions(id);
    toast('Making captions on this computer. It takes a few minutes for each hour of speech.');
    hooks.render();
    clearTimeout(pollTimer);
    pollTimer = setTimeout(load, 2000);
  } catch (err) {
    toast(err.message, true);
  }
}
