// The Watching view: channels and playlists YTGrab follows.
import { $, el, formatWhen, ICONS, stateBlock, toast } from './ui.js';

// Set by initWatching: { client, presetLabels, setNavLabel, refreshJobs }.
let deps;

export function initWatching(dependencies) {
  deps = dependencies;
}

let watchInfo = null; // { watches, interval_hours, ... }
const watchBusy = new Set();
let feedOpen = null; // { id, info: { url, local_url, reason } } for the watch showing its feed

const FEED_ICON = '<path d="M5 11a8 8 0 0 1 8 8M5 5a14 14 0 0 1 14 14"/><circle cx="6" cy="18" r="1.5"/>';

// A watch's podcast feed: its address, and which apps can use it.
function feedPanel(w) {
  const { info } = feedOpen;
  const address = (value, label) => {
    const input = el('input', { type: 'text', readOnly: true, value, className: 'feed-url' });
    input.setAttribute('aria-label', label);
    input.addEventListener('focus', () => input.select());
    return input;
  };
  const panel = el('div', { className: 'watch-feed' });
  if (info.url) {
    const copy = el('button', { type: 'button', className: 'btn btn-small', textContent: 'Copy' });
    copy.dataset.watchAction = 'copy-feed';
    panel.append(
      el('strong', { textContent: 'Podcast feed' }),
      el('div', { className: 'feed-row' }, address(info.url, `Podcast feed address for ${w.title}`), copy),
      el('p', {
        className: 'hint',
        textContent:
          'Add it in a podcast app that refreshes on the phone itself, such as AntennaPod or Podcast Addict, while on the same Wi-Fi. Apps that refresh through their own servers (Pocket Casts, Spotify) can’t reach this computer. Episodes play best when the watch saves M4A or MP3.',
      }),
    );
  } else {
    panel.append(el('strong', { textContent: 'Podcast feed' }), el('p', { className: 'hint', textContent: info.reason || 'The feed isn’t available right now.' }));
    if (info.local_url) panel.append(el('p', { className: 'hint', textContent: 'For a podcast app on this computer:' }), address(info.local_url, `Podcast feed address on this computer for ${w.title}`));
  }
  return panel;
}

export async function loadWatches() {
  try {
    watchInfo = await deps.client.listWatches();
  } catch (err) {
    if (err.code === 'not_available') return; // older server
    $('#watch-list').replaceChildren(stateBlock('error', "Couldn't load watched channels", err.message));
    return;
  }
  renderWatches();
}

export function renderWatches() {
  const list = $('#watch-list');
  list.setAttribute('aria-busy', 'false');
  const watches = watchInfo?.watches || [];
  const active = watches.filter((w) => !w.paused).length;
  $('#watching-nav-count').textContent = active || '';
  deps.setNavLabel('watching', 'Watching', active, active === 1 ? 'channel or playlist' : 'channels and playlists');
  if (watchInfo?.interval_hours) {
    $('#watching-sub').textContent = `YTGrab checks these every ${watchInfo.interval_hours} hours while it's running and downloads new videos as they appear.`;
  }
  if (!watches.length) {
    list.replaceChildren(stateBlock('empty', 'Not watching anything yet', 'Add a channel or playlist above, and its new videos will download by themselves.'));
    return;
  }
  list.replaceChildren(
    ...watches.map((w) => {
      const every = { 1: 'every hour', 24: 'daily' }[w.interval_hours] || 'every 6 hours';
      const meta = [
        w.kind === 'channel' ? 'Channel' : 'Playlist',
        deps.presetLabels[w.preset] || w.preset,
        w.min_minutes ? `${w.min_minutes}+ min` : '',
        w.keywords ? `titles with “${w.keywords}”` : '',
        w.paused ? '' : every,
        w.paused ? 'paused' : w.last_checked ? `checked ${formatWhen(w.last_checked)}` : 'not checked yet',
        w.downloaded ? `${w.downloaded} downloaded` : '',
      ].filter(Boolean);
      const busy = watchBusy.has(w.id);
      const button = (action, label) => {
        const b = el('button', { type: 'button', className: 'act', title: label, disabled: busy });
        b.dataset.watchAction = action;
        b.setAttribute('aria-label', `${label}: ${w.title}`);
        const icon = action === 'feed' ? FEED_ICON : ICONS[action === 'check' ? 'retry' : action === 'unpause' ? 'resume' : action === 'pause' ? 'pause' : 'remove'];
        b.innerHTML = `<svg viewBox="0 0 24 24" aria-hidden="true">${icon}</svg>`;
        if (action === 'feed') b.setAttribute('aria-expanded', String(feedOpen?.id === w.id));
        return b;
      };
      const card = el(
        'article',
        { className: 'watch' },
        el('div', { className: 'watch-avatar', textContent: (w.title || '?').trim().charAt(0).toUpperCase() }),
        el(
          'div',
          { className: 'watch-main' },
          el('h3', { className: 'watch-title' }, el('span', { textContent: w.title, title: w.url })),
          el('p', { className: 'watch-meta', textContent: meta.join(' · ') }),
          ...(w.last_error ? [el('p', { className: 'watch-error', textContent: w.last_error })] : []),
        ),
        el('div', { className: 'watch-actions-row' }, button('feed', 'Podcast feed'), button('check', 'Check now'), button(w.paused ? 'unpause' : 'pause', w.paused ? 'Resume watching' : 'Pause watching'), button('remove', 'Stop watching')),
      );
      if (feedOpen?.id === w.id) card.append(feedPanel(w));
      card.dataset.id = w.id;
      if (w.paused) card.dataset.paused = 'true';
      return card;
    }),
  );
}

export async function onWatchAction(id, action) {
  const watch = watchInfo?.watches.find((w) => w.id === id);
  if (!watch) return;
  if (action === 'feed') {
    if (feedOpen?.id === id) {
      feedOpen = null;
    } else {
      try {
        feedOpen = { id, info: await deps.client.watchFeed(id) };
      } catch (err) {
        toast(err.message, true);
        return;
      }
    }
    renderWatches();
    return;
  }
  if (action === 'copy-feed') {
    const url = feedOpen?.info.url;
    try {
      await navigator.clipboard.writeText(url);
      toast('Feed address copied. Add it in your podcast app.');
    } catch {
      // Browsers only allow copying on secure pages, which a phone's view of YTGrab isn't.
      const input = document.querySelector('.watch-feed .feed-url');
      input?.focus();
      toast('Press and hold the selected address to copy it.');
    }
    return;
  }
  if (action === 'remove' && !confirm(`Stop watching ${watch.title}? Videos it already downloaded are kept.`)) return;
  watchBusy.add(id);
  renderWatches();
  try {
    if (action === 'check') {
      const checked = await deps.client.checkWatch(id);
      toast(checked.last_new ? `Found ${checked.last_new} new video${checked.last_new === 1 ? '' : 's'} and added ${checked.last_new === 1 ? 'it' : 'them'} to the queue.` : `No new videos from ${watch.title}.`);
    } else if (action === 'remove') {
      await deps.client.deleteWatch(id);
      toast(`Stopped watching ${watch.title}.`);
    } else {
      const paused = action === 'pause';
      await deps.client.updateWatch(id, { paused });
      toast(paused ? `Paused watching ${watch.title}.` : `Watching ${watch.title} again.`);
    }
  } catch (err) {
    toast(err.message, true);
  } finally {
    watchBusy.delete(id);
    await loadWatches();
  }
}

export async function onWatchSubmit(e) {
  e.preventDefault();
  const url = $('#watch-url').value.trim();
  const error = $('#watch-error');
  if (!url) {
    error.textContent = 'Paste a link to a YouTube channel or playlist, or a Vimeo channel, group, or showcase first.';
    error.hidden = false;
    return;
  }
  const button = $('#watch-add');
  button.disabled = true;
  button.textContent = 'Reading the channel…';
  error.hidden = true;
  try {
    const watch = await deps.client.addWatch({
      url,
      preset: $('#watch-preset').value,
      folder: $('#watch-folder').checked,
      backfill: Number($('#watch-backfill').value),
      min_minutes: Number($('#watch-min').value),
      keywords: $('#watch-keywords').value.trim(),
      interval_hours: Number($('#watch-interval').value),
    });
    $('#watch-keywords').value = '';
    $('#watch-url').value = '';
    toast(watch.last_new ? `Watching ${watch.title}. Added ${watch.last_new} video${watch.last_new === 1 ? '' : 's'} to the queue.` : `Watching ${watch.title}. New videos will download as they appear.`);
    await deps.refreshJobs();
    await loadWatches();
  } catch (err) {
    error.textContent = err.message;
    error.hidden = false;
  } finally {
    button.disabled = false;
    button.textContent = 'Start watching';
  }
}
