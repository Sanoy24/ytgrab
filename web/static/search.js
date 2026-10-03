// Searching YouTube from the link field.
import { formatDuration } from './formats.js';
import { $, el, setThumbnail } from './ui.js';

// Set by initSearch: { client, open(link) }.
let deps;

export function initSearch(dependencies) {
  deps = dependencies;
}

let searchCtl = null;

export function clearSearch() {
  searchCtl?.abort();
  searchCtl = null;
  $('#search-results').hidden = true;
  $('#search-results').replaceChildren();
}

export async function runSearch(query) {
  searchCtl?.abort();
  const ctl = (searchCtl = new AbortController());
  const box = $('#search-results');
  box.hidden = false;
  $('#search-hint').hidden = true;
  box.replaceChildren(el('p', { className: 'search-status' }, el('span', { className: 'spinner' }), `Searching YouTube for “${query}”…`));
  try {
    const { results } = await deps.client.search(query, { signal: ctl.signal });
    if (ctl.signal.aborted) return;
    if (!results.length) {
      box.replaceChildren(el('p', { className: 'search-status', textContent: `Nothing found for “${query}”.` }));
      return;
    }
    box.replaceChildren(
      ...results.map((r) => {
        const img = el('img', { alt: '', referrerPolicy: 'no-referrer', loading: 'lazy' });
        setThumbnail(img, r.video_id);
        const button = el(
          'button',
          { type: 'button', className: 'search-result' },
          img,
          el('span', {}, el('strong', { textContent: r.title }), el('small', { textContent: [r.channel, formatDuration(r.duration_seconds)].filter(Boolean).join(' · ') })),
        );
        button.addEventListener('click', () => {
          clearSearch();
          deps.open(`https://www.youtube.com/watch?v=${r.video_id}`);
        });
        return button;
      }),
    );
  } catch (err) {
    if (err.name === 'AbortError' || ctl.signal.aborted) return;
    box.replaceChildren(el('p', { className: 'search-status inspect-error', textContent: err.message }));
  }
}
