// Sends a page's or a link's address to YTGrab, which runs on this computer at
// http://127.0.0.1:<port>/ and puts a ?url= address into its link field. Nothing is
// downloaded until Add is pressed there. The extension reads no page content.

const api = globalThis.browser ?? globalThis.chrome;
const DEFAULT_PORT = 8787;

async function base() {
  const { port } = await api.storage.sync.get({ port: DEFAULT_PORT });
  return `http://127.0.0.1:${port}/`;
}

// YTGrab answers every request with this header, so another program on the port isn't
// mistaken for it.
async function running(root) {
  try {
    const response = await fetch(`${root}api/system/version`, { cache: 'no-store' });
    return response.ok && response.headers.has('X-YTGrab-Version');
  } catch {
    return false;
  }
}

async function showProblem(title) {
  await api.action.setBadgeBackgroundColor({ color: '#d1352f' });
  await api.action.setBadgeText({ text: '!' });
  await api.action.setTitle({ title });
}

async function clearProblem() {
  await api.action.setBadgeText({ text: '' });
  await api.action.setTitle({ title: 'Download with YTGrab' });
}

// X's timeline and profiles hold many posts, so their address isn't any one post's.
function notAPost(address) {
  try {
    const { hostname, pathname } = new URL(address);
    return /(^|\.)(x|twitter)\.com$/i.test(hostname) && !/\/status\/\d+/.test(pathname);
  } catch {
    return false;
  }
}

async function send(address) {
  if (!/^https?:\/\//i.test(address || '')) {
    await showProblem('Open a video page first, then click Download with YTGrab.');
    return;
  }
  if (notAPost(address)) {
    await showProblem("This is X's timeline, not a post. Right-click the post's date (like \"Oct 2\") and choose Download link with YTGrab, or open the post first.");
    return;
  }
  const root = await base();
  if (!(await running(root))) {
    await showProblem(`YTGrab isn't running at ${root}. Start it, then try again. (The port can be changed in this extension's options.)`);
    return;
  }
  await clearProblem();
  const target = `${root}?url=${encodeURIComponent(address)}`;
  // Reuse an open YTGrab tab, so repeated sends don't pile up tabs.
  const [open] = await api.tabs.query({ url: `${root}*` });
  if (open) {
    await api.tabs.update(open.id, { url: target, active: true });
    await api.windows.update(open.windowId, { focused: true });
  } else {
    await api.tabs.create({ url: target });
  }
}

api.action.onClicked.addListener((tab) => send(tab.url));

api.runtime.onInstalled.addListener(() => {
  api.contextMenus.create({ id: 'ytgrab-link', title: 'Download link with YTGrab', contexts: ['link'] });
  api.contextMenus.create({ id: 'ytgrab-page', title: 'Download with YTGrab', contexts: ['page', 'video'] });
});

api.contextMenus.onClicked.addListener((info, tab) => {
  send(info.menuItemId === 'ytgrab-link' ? info.linkUrl : info.pageUrl || tab?.url);
});
