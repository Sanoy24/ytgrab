// Settings → Phone: phone access on the same Wi-Fi, pairing by QR code, and paired phones.
// Shown only on the computer; phones can't change it.
import { $, el, formatWhen, toast } from './ui.js';

let client;
let openSettings; // shows Settings → Phone
let status = null; // { enabled, url, error, moved, devices }
let pollTimer = null;
let codeTimer = null;

export function initPhone(apiClient, showPhoneSettings) {
  client = apiClient;
  openSettings = showPhoneSettings;
  $('#phone-moved-fix').addEventListener('click', () => {
    openSettings();
    if ($('#phone-qr').hidden) $('#phone-pair').click();
  });
  // The computer's address can change any time the router hands out a new one.
  setInterval(loadPhone, 60_000);
  $('#pref-phone').addEventListener('change', onToggle);
  $('#phone-pair').addEventListener('click', showCode);
  $('#phone-devices').addEventListener('click', (e) => {
    const b = e.target.closest('button[data-device]');
    if (b) forget(b, b.dataset.device);
  });
  loadPhone();
}

export async function loadPhone() {
  try {
    status = await client.phoneStatus();
  } catch {
    $('#set-phone').hidden = true; // an older server
    return;
  }
  render();
}

function render() {
  $('#set-phone').hidden = false;
  $('#phone-moved-banner').hidden = !status.moved;
  $('#phone-moved').hidden = !status.moved;
  $('#pref-phone').checked = status.enabled;
  $('#phone-pair-row').hidden = !status.enabled;
  $('#phone-error').hidden = !status.error;
  $('#phone-error').textContent = status.error || '';
  $('#phone-url').textContent = status.url || '';
  $('#phone-address').hidden = !status.url;
  $('#phone-devices-row').hidden = !status.enabled || !status.devices.length;
  $('#phone-devices').replaceChildren(
    ...status.devices.map((d) => {
      const remove = el('button', { type: 'button', className: 'btn btn-small', textContent: 'Remove' });
      remove.dataset.device = d.id;
      remove.setAttribute('aria-label', `Remove ${d.name}`);
      return el(
        'li',
        { className: 'phone-device' },
        el('span', { className: 'phone-device-name', textContent: d.name }),
        el('span', { className: 'phone-device-when', textContent: d.moved ? "Can't connect: scan a new code" : `Paired ${formatWhen(d.paired_at)}` }),
        remove,
      );
    }),
  );
  if (!status.enabled) hideCode();
}

async function onToggle(e) {
  const box = e.currentTarget;
  box.disabled = true;
  try {
    status = await client.setPhoneAccess(box.checked);
    render();
    toast(status.enabled ? 'Phone access is on. Pair a phone with the code.' : 'Phone access is off, and paired phones were removed.');
  } catch (err) {
    box.checked = !box.checked;
    toast(err.message, true);
  } finally {
    box.disabled = false;
  }
}

async function showCode(e) {
  const button = e.currentTarget;
  button.disabled = true;
  try {
    const pairing = await client.pairPhone();
    const img = $('#phone-qr');
    img.src = pairing.qr;
    img.hidden = false;
    $('#phone-code-link').textContent = pairing.url;
    $('#phone-code-link').hidden = false;
    button.textContent = 'New code';
    // Watch for the phone to pair, then put the code away.
    const before = status.devices.length;
    const moved = status.moved; // a phone pairing again replaces its old entry
    clearInterval(pollTimer);
    pollTimer = setInterval(async () => {
      try {
        status = await client.phoneStatus();
      } catch {
        return;
      }
      if (status.devices.length > before || (moved && !status.moved)) {
        render();
        hideCode();
        toast(`${status.devices.at(-1).name} is paired.`);
      }
    }, 2000);
    clearTimeout(codeTimer);
    codeTimer = setTimeout(hideCode, new Date(pairing.expires_at) - Date.now());
  } catch (err) {
    toast(err.message, true);
  } finally {
    button.disabled = false;
  }
}

function hideCode() {
  clearInterval(pollTimer);
  clearTimeout(codeTimer);
  $('#phone-qr').hidden = true;
  $('#phone-qr').removeAttribute('src');
  $('#phone-code-link').hidden = true;
  $('#phone-pair').textContent = 'Show a code';
}

async function forget(button, id) {
  button.disabled = true;
  try {
    await client.forgetPhone(id);
    status = await client.phoneStatus();
    render();
    toast('Phone removed. It can no longer use YTGrab.');
  } catch (err) {
    button.disabled = false;
    toast(err.message, true);
  }
}
