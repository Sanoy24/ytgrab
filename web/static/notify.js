// Settings → Notifications: finished and failed downloads, sent to ntfy, Discord, or a
// webhook, so they reach a phone even away from home.
import { $, toast } from './ui.js';

let client;

const HELP = {
  '': 'Off: nothing is sent anywhere.',
  ntfy: 'Install the free ntfy app, subscribe to a topic name only you know (it works like a password), and enter https://ntfy.sh/ followed by that name.',
  discord: 'In Discord: channel settings → Integrations → Webhooks → New webhook → Copy Webhook URL.',
  webhook: 'YTGrab sends a JSON POST with app, event, title, and message to this address.',
};

export function initNotify(apiClient) {
  client = apiClient;
  $('#notify-kind').addEventListener('change', syncFields);
  $('#notify-form').addEventListener('submit', save);
  $('#notify-test').addEventListener('click', test);
}

// Called with the settings as they load: { notify: { kind, url, events } }.
export function applyNotifySettings(settings) {
  const current = settings?.notify;
  if (!current || document.activeElement?.closest?.('#notify-form')) return; // don't undo typing
  $('#notify-kind').value = current.kind || '';
  $('#notify-url').value = current.url || '';
  $('#notify-events').value = current.events || 'all';
  syncFields();
}

function syncFields() {
  const kind = $('#notify-kind').value;
  for (const id of ['#notify-url-row', '#notify-events-row', '#notify-test']) $(id).hidden = !kind;
  $('#notify-help').textContent = HELP[kind] ?? '';
  $('#notify-url').placeholder = { ntfy: 'https://ntfy.sh/your-secret-topic', discord: 'https://discord.com/api/webhooks/…', webhook: 'https://…' }[kind] || '';
}

function current() {
  const kind = $('#notify-kind').value;
  return { kind, url: kind ? $('#notify-url').value.trim() : '', events: $('#notify-events').value };
}

async function save(e) {
  e.preventDefault();
  const button = $('#notify-save');
  button.disabled = true;
  try {
    const saved = await client.setNotify(current());
    toast(saved.kind ? 'Notifications saved. Send a test to see one arrive.' : 'Notifications are off.');
  } catch (err) {
    toast(err.message, true);
  } finally {
    button.disabled = false;
  }
}

async function test(e) {
  const button = e.currentTarget;
  button.disabled = true;
  try {
    await client.testNotify(current());
    toast('Test sent. It should arrive in a few seconds.');
  } catch (err) {
    toast(err.message, true);
  } finally {
    button.disabled = false;
  }
}
