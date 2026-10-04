// Client-side checks for pasted links. They give fast feedback only; the server validates
// every link again before using it.

// X (Twitter) posts with a video: x.com/<user>/status/<id>.
// Vimeo videos: vimeo.com/<id>, unlisted vimeo.com/<id>/<hash>, channel and group links, and
// the embedded player.
const VIMEO_HOSTS = new Set(['vimeo.com', 'www.vimeo.com', 'player.vimeo.com']);

// Instagram posts and reels: instagram.com/reel/<code>/, /p/<code>/, /<user>/reel/<code>/.
const INSTAGRAM_HOSTS = new Set(['instagram.com', 'www.instagram.com', 'm.instagram.com']);

// Reddit posts with a video: reddit.com/r/<sub>/comments/<id>/, redd.it/<id>, v.redd.it/<id>.
const REDDIT_HOSTS = new Set(['reddit.com', 'www.reddit.com', 'old.reddit.com', 'new.reddit.com', 'm.reddit.com', 'np.reddit.com', 'sh.reddit.com']);

const X_HOSTS = new Set(['x.com', 'www.x.com', 'mobile.x.com', 'twitter.com', 'www.twitter.com', 'mobile.twitter.com']);

const YT_HOSTS = new Set([
  'youtube.com',
  'www.youtube.com',
  'm.youtube.com',
  'music.youtube.com',
  'youtu.be',
]);

// Vimeo links become the embedded player's link, as the server writes them: the player
// plays public and unlisted videos without signing in.
function vimeoLink(u) {
  const parts = u.pathname.split('/').filter(Boolean);
  let hash = u.searchParams.get('h') || '';
  let id = '';
  if (u.hostname.toLowerCase() === 'player.vimeo.com') {
    if (parts.length === 2 && parts[0] === 'video') id = parts[1];
  } else if (/^\d{5,12}$/.test(parts[0] || '')) {
    id = parts[0];
    if (parts.length === 2 && !hash) hash = parts[1];
  } else if (parts.length >= 3) {
    id = parts[parts.length - 1];
  }
  if (!/^\d{5,12}$/.test(id) || (hash && !/^[0-9a-f]{6,20}$/.test(hash)))
    return { error: "This Vimeo link doesn't point to a video. Open the video and copy its link." };
  return { url: `https://player.vimeo.com/video/${id}${hash ? `?h=${hash}` : ''}`, site: 'vimeo', id };
}

// Instagram links become one form per post, as the server writes them; ?item=N is YTGrab's
// own link to one video of a carousel (photo positions like ?img_index= are ignored).
function instagramLink(u) {
  const code = (u.pathname.match(/\/(?:p|reels?|tv)\/([A-Za-z0-9_-]{5,40})(?:\/|$)/) || [])[1];
  if (!code) return { error: "This Instagram link doesn't point to a post or reel. Open it and copy its link." };
  const n = Number(u.searchParams.get('item'));
  const index = Number.isInteger(n) && n >= 2 && n <= 20 ? n : 1;
  const url = `https://www.instagram.com/p/${code}/${index > 1 ? `?item=${index}` : ''}`;
  return { url, site: 'instagram', id: index > 1 ? `${code}.${index}` : code };
}

// Reddit links become one form per post (or v.redd.it video), as the server writes them.
function redditLink(host, path) {
  if (host === 'v.redd.it') {
    const id = (path.match(/^\/([a-z0-9]{6,20})\/?$/) || [])[1];
    return id ? { url: `https://v.redd.it/${id}`, site: 'reddit', id } : { error: "This link doesn't point to a Reddit video." };
  }
  const id = host === 'redd.it' ? (path.match(/^\/([a-z0-9]{4,10})\/?$/) || [])[1] : (path.match(/\/comments\/([a-z0-9]{4,10})(?:\/|$)/) || [])[1];
  if (id) return { url: `https://www.reddit.com/comments/${id}`, site: 'reddit', id };
  if (/\/s\/[A-Za-z0-9]+\/?$/.test(path))
    return { error: "Reddit's share links can't be read directly. Open the post, then copy the link from the address bar." };
  return { error: "This Reddit link doesn't point to a post. Open the post and copy its link." };
}

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
  const host = u.hostname.toLowerCase();
  if (X_HOSTS.has(host)) {
    const match = u.pathname.match(/\/status\/(\d{5,20})(?:\/video\/(\d+))?(?:\/|$)/);
    if (!match) return { error: "This X link doesn't point to a post. Open the post, or right-click its date and send or copy that link." };
    // /video/N picks one video of a post with several; the first is the post itself.
    const n = Number(match[2]);
    const index = n >= 2 && n <= 4 ? n : 1;
    const url = `https://x.com/i/status/${match[1]}${index > 1 ? `/video/${index}` : ''}`;
    return { url, site: 'x', id: index > 1 ? `${match[1]}-${index}` : match[1] };
  }
  if (REDDIT_HOSTS.has(host) || host === 'redd.it' || host === 'v.redd.it') return redditLink(host, u.pathname);
  if (INSTAGRAM_HOSTS.has(host)) return instagramLink(u);
  if (VIMEO_HOSTS.has(host)) return vimeoLink(u);
  if (!YT_HOSTS.has(host))
    return { error: 'Only YouTube, Vimeo, X, Reddit, and Instagram links are supported.' };

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

const VIDEO_ID = /^[A-Za-z0-9_-]{11}$/;

// The 11-character video ID of a single-video link checked by validateUrl, or ''.
export function videoIdOf(url) {
  const u = new URL(url);
  const id =
    u.hostname.toLowerCase() === 'youtu.be'
      ? u.pathname.slice(1).split('/')[0]
      : u.searchParams.get('v') || (u.pathname.match(/^\/(?:shorts|live|embed)\/([^/]+)/) || [])[1] || '';
  return VIDEO_ID.test(id) ? id : '';
}

// Splits pasted text into links. Returns null for a single link (or nothing), so the
// usual one-link flow handles it; otherwise { videos: [{ id, url }], skipped }, where
// videos are unique single-video links in order and skipped counts everything else
// (playlists, other sites, typos).
export function parseLinks(text) {
  const tokens = text.split(/\s+/).filter(Boolean);
  if (tokens.length < 2) return null;
  const videos = [];
  const seen = new Set();
  let skipped = 0;
  for (const token of tokens) {
    const result = validateUrl(token);
    const id = result.url ? result.id || videoIdOf(result.url) : '';
    if (!id) {
      skipped++;
    } else if (!seen.has(id)) {
      seen.add(id);
      videos.push({ id, url: result.url });
    }
  }
  return { videos, skipped };
}

// Words typed instead of a link, to search YouTube for: anything that isn't a link and
// doesn't mention a web address.
export function looksLikeSearch(text) {
  const value = text.trim();
  if (value.length < 2 || /(https?:\/\/|www\.|youtu\.be|youtube\.|\.com\b)/i.test(value)) return false;
  return Boolean(validateUrl(value).error);
}
