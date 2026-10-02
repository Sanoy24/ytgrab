// Client-side checks for pasted links. They give fast feedback only; the server validates
// every link again before using it.

const YT_HOSTS = new Set([
  'youtube.com',
  'www.youtube.com',
  'm.youtube.com',
  'music.youtube.com',
  'youtu.be',
]);

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
