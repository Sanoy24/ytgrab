// Data clients. The UI talks only to this interface:
//   health(), listJobs(), createJob({url, preset}), cancelJob(id), retryJob(id), subscribe(onChange),
//   getSettings(), updateSettings({downloads_dir}), inspect(url, {signal}),
//   listPlaylist(url, {signal}), createPlaylistJobs({video_ids, preset})
// The live HTTP client is the default. Add ?fixture=default|empty|error|degraded|loading
// to preview UI states without the backend.

import * as fx from './fixtures.js';

export class ApiError extends Error {
  constructor(code, message) {
    super(message);
    this.code = code;
  }
}

export function createClient(params = new URLSearchParams(location.search)) {
  if (params.has('fixture')) return new FixtureClient(params.get('fixture') || 'default');
  return new HttpClient();
}

// Scenarios: default | empty | error | degraded | loading | blocked (format check fails) | expired (first format job needs a re-check) | first-run (no folder chosen) | no-picker (no folder window) | cooldown (YouTube pause) | outdated (yt-dlp update available)
class FixtureClient {
  isFixture = true;

  constructor(scenario) {
    this.scenario = scenario;
    this.jobs = scenario === 'empty' ? [] : structuredClone(fx.jobs);
    this.nextId = 100;
    this.downloadsDir = String.raw`C:\Users\me\Downloads\ytgrab`;
    this.listeners = new Set();
    this.timer = null;
  }

  async version() {
    await delay(150);
    const update = this.scenario === 'default';
    return {
      version: '1.7.0',
      latest: update ? '1.8.0' : '1.7.0',
      update_available: update,
      update_command: update ? 'scoop update ytgrab' : undefined,
      release_url: 'https://github.com/Sanoy24/ytgrab/releases/latest',
    };
  }

  async health() {
    await delay(150);
    if (this.scenario === 'error')
      throw new ApiError('unreachable', 'Could not reach the local server.');
    if (this.scenario === 'outdated' && !this.ytdlpUpdated) return fx.healthOutdated;
    return this.scenario === 'degraded' ? fx.healthDegraded : fx.healthOk;
  }

  async listJobs() {
    if (this.scenario === 'cooldown' && this.pausedUntil === undefined) {
      this.pausedUntil = new Date(Date.now() + 14 * 60_000 + 32_000).toISOString();
    }
    await delay(this.scenario === 'loading' ? 1e9 : 300);
    if (this.scenario === 'error') {
      throw new ApiError(
        'unreachable',
        'Could not reach the local server. Is ytgrab still running?',
      );
    }
    return structuredClone(this.jobs);
  }

  async resume() {
    await delay(200);
    this.pausedUntil = null;
  }

  async revealJob(id) {
    await delay(200);
    const job = this.jobs.find((j) => j.id === id);
    if (!job?.output_path) throw new ApiError('file_missing', 'The file was moved or deleted.');
  }

  async deleteJob(id) {
    await delay(200);
    const job = this.jobs.find((j) => j.id === id);
    if (!job) throw new ApiError('not_found', 'That job no longer exists.');
    if (isActive(job.state)) throw new ApiError('invalid_state', 'Cancel the download before removing it.');
    this.jobs = this.jobs.filter((j) => j.id !== id);
    this.emit();
  }

  async clearHistory() {
    await delay(200);
    const before = this.jobs.length;
    this.jobs = this.jobs.filter((j) => isActive(j.state));
    this.emit();
    return { removed: before - this.jobs.length };
  }

  async updateYtdlp() {
    await delay(2000);
    this.ytdlpUpdated = true;
    return { version: '2026.09.30', previous: '2026.08.19', updated: true };
  }

  async inspect(url, { signal } = {}) {
    await delay(this.scenario === 'loading' ? 1e9 : 1500, signal);
    if (this.scenario === 'error')
      throw new ApiError('unreachable', 'Could not reach the local server.');
    if (this.scenario === 'blocked') {
      throw new ApiError(
        'blocked',
        'YouTube is limiting requests from this network. Wait a while, then retry.',
      );
    }
    if (url.includes('xxxxxxxxxxx'))
      throw new ApiError('video_unavailable', 'This video is unavailable or private.');
    return structuredClone(fx.inspection);
  }

  async listPlaylist(url, { signal, start = 1 } = {}) {
    await delay(start > 1 ? 600 : 1200, signal);
    if (this.scenario === 'error')
      throw new ApiError('unreachable', 'Could not reach the local server.');
    if (this.scenario === 'blocked') {
      throw new ApiError(
        'blocked',
        'YouTube is limiting requests from this network. Wait a while, then retry.',
      );
    }
    if (url.includes('list=PLbig')) {
      // A 230-video playlist, served in pages of 50.
      const total = 230;
      const entries = Array.from({ length: Math.max(0, Math.min(50, total - start + 1)) }, (_, i) => ({
        video_id: `big${String(start + i).padStart(8, '0')}`,
        title: `Episode ${start + i}`,
        duration_seconds: 600 + ((start + i) % 7) * 60,
      }));
      const next = start + 50 <= total ? start + 50 : null;
      return { id: 'PLbig', title: 'A very long series', entries, total, start, next, truncated: next !== null, unavailable: 0 };
    }
    return { ...structuredClone(fx.playlist), start: 1, next: null };
  }

  async createPlaylistJobs({ video_ids, preset }) {
    await delay(500);
    const created = [];
    let skipped = 0;
    for (const id of new Set(video_ids)) {
      const url = `https://www.youtube.com/watch?v=${id}`;
      if (this.jobs.some((j) => j.url === url && isActive(j.state))) {
        skipped++;
        continue;
      }
      const entry = fx.playlist.entries.find((e) => e.video_id === id);
      const at = new Date().toISOString();
      const job = {
        id: `job_${this.nextId++}`, url, video_id: id, title: entry?.title ?? null, preset, format: null,
        state: 'queued', attempt: 1, progress: null, output_path: null, error: null, created_at: at, updated_at: at,
      };
      this.jobs.unshift(job);
      created.push(job);
    }
    skipped += video_ids.length - new Set(video_ids).size;
    this.emit();
    return { jobs: structuredClone(created), skipped };
  }

  async createJob({ url, preset, format }) {
    await delay(400);
    if (this.scenario === 'error')
      throw new ApiError('unreachable', 'Could not reach the local server.');
    // Simulates the server forgetting an inspection: the first format job is rejected.
    if (this.scenario === 'expired' && format && !this.expiredOnce) {
      this.expiredOnce = true;
      throw new ApiError(
        'inspection_required',
        'Check available formats again before adding this download.',
      );
    }
    if (this.jobs.some((j) => j.url === url && isActive(j.state))) {
      throw new ApiError('duplicate_job', 'This link is already in the queue.');
    }
    const at = new Date().toISOString();
    const job = {
      id: `job_${this.nextId++}`,
      url,
      video_id: null,
      title: null,
      preset: format ? null : preset,
      format: format ? { ...format, label: fixtureFormatLabel(format) } : null,
      state: 'queued',
      attempt: 1,
      progress: null,
      output_path: null,
      error: null,
      created_at: at,
      updated_at: at,
    };
    this.jobs.unshift(job);
    this.emit();
    return structuredClone(job);
  }

  async cancelJob(id) {
    await delay(200);
    this.update(id, (j) => {
      if (!isActive(j.state))
        throw new ApiError('invalid_state', 'Only queued or running jobs can be cancelled.');
      j.state = 'cancelled';
    });
  }

  async retryJob(id) {
    await delay(200);
    this.update(id, (j) => {
      if (j.state !== 'failed' && j.state !== 'cancelled') {
        throw new ApiError('invalid_state', 'Only failed or cancelled jobs can be retried.');
      }
      Object.assign(j, { state: 'queued', attempt: j.attempt + 1, error: null, progress: null });
    });
  }

  settingsBody() {
    return {
      downloads_dir: this.downloadsDir,
      configured: this.configured ?? this.scenario !== 'first-run',
      default_dir: String.raw`C:\Users\me\Downloads\ytgrab`,
      can_pick: this.scenario !== 'no-picker',
      cookies_browser: this.cookiesBrowser ?? '',
      max_downloads: this.maxDownloads ?? 2,
      max_downloads_limit: 4,
      default_preset: this.defaultPreset ?? 'video-best',
      start_at_login: this.startAtLogin ?? false,
      speed_limit_kbps: this.speedLimit ?? 0,
      speed_limits: [0, 500, 1000, 2000, 5000, 10000],
      subtitles_mode: this.subtitlesMode ?? 'off',
      subtitles_lang: this.subtitlesLang ?? 'en',
      subtitle_languages: ['en', 'am', 'ar', 'de', 'es', 'fr', 'hi', 'id', 'it', 'ja', 'ko', 'nl', 'pl', 'pt', 'ru', 'sw', 'tr', 'uk', 'vi', 'zh'],
      cookie_browsers: ['firefox', 'chrome', 'edge', 'brave', 'chromium', 'opera', 'vivaldi', 'safari'],
    };
  }

  async setPreferences({ max_downloads, default_preset, speed_limit_kbps }) {
    await delay(200);
    if (speed_limit_kbps !== undefined) this.speedLimit = speed_limit_kbps;
    if (max_downloads !== undefined) this.maxDownloads = max_downloads;
    if (default_preset !== undefined) this.defaultPreset = default_preset;
    return this.settingsBody();
  }

  async setSubtitles({ mode, lang }) {
    await delay(200);
    this.subtitlesMode = mode;
    this.subtitlesLang = lang;
    return this.settingsBody();
  }

  async setStartAtLogin(enabled) {
    await delay(200);
    this.startAtLogin = enabled;
    return this.settingsBody();
  }

  async setCookiesBrowser(browser) {
    await delay(200);
    this.cookiesBrowser = browser;
    return this.settingsBody();
  }

  async getSettings() {
    await delay(200);
    if (this.scenario === 'error')
      throw new ApiError('unreachable', 'Could not reach the local server.');
    return this.settingsBody();
  }

  async updateSettings({ downloads_dir }) {
    await delay(300);
    // Loosely mirrors the server rule: an absolute path to an existing, writable folder.
    if (!/^([a-z]:[\\/]|\\\\|\/)/i.test(downloads_dir)) {
      throw new ApiError('invalid_directory', 'Choose an existing, writable absolute folder.');
    }
    this.downloadsDir = downloads_dir;
    this.configured = true;
    return this.settingsBody();
  }

  // Simulates the desktop folder window: the first pick is cancelled, later ones succeed.
  async pickFolder() {
    await delay(1500);
    if (this.scenario === 'no-picker') {
      throw new ApiError('picker_unavailable', 'No folder window is available here. Type the folder path instead.');
    }
    this.picks = (this.picks ?? 0) + 1;
    if (this.picks === 1 && this.scenario === 'first-run') return { ...this.settingsBody(), cancelled: true };
    this.downloadsDir = String.raw`D:\Videos\YouTube`;
    this.configured = true;
    return this.settingsBody();
  }

  async useDefaultFolder() {
    await delay(300);
    this.downloadsDir = String.raw`C:\Users\me\Downloads\ytgrab`;
    this.configured = true;
    return this.settingsBody();
  }

  update(id, fn) {
    const job = this.jobs.find((j) => j.id === id);
    if (!job) throw new ApiError('not_found', 'That job no longer exists.');
    fn(job);
    job.updated_at = new Date().toISOString();
    this.emit();
  }

  // Simulates SSE progress: advances downloads and starts queued jobs, two at a time.
  subscribe(onChange) {
    this.listeners.add(onChange);
    if (!this.timer && !['error', 'loading'].includes(this.scenario))
      this.timer = setInterval(() => this.tick(), 1000);
    return () => {
      this.listeners.delete(onChange);
      if (!this.listeners.size) (clearInterval(this.timer), (this.timer = null));
    };
  }

  tick() {
    let changed = false;
    for (const j of this.jobs) {
      if (j.state === 'downloading') {
        const p = j.progress;
        p.downloaded_bytes += p.speed_bps;
        if (p.total_bytes != null) {
          p.eta_seconds = Math.max(
            0,
            Math.round((p.total_bytes - p.downloaded_bytes) / p.speed_bps),
          );
          if (p.downloaded_bytes >= p.total_bytes) {
            p.downloaded_bytes = p.total_bytes;
            j.state = 'processing';
          }
        }
        changed = true;
      } else if (j.state === 'processing' && Math.random() < 0.4) {
        j.state = 'completed';
        j.output_path = `C:\\Users\\me\\Downloads\\ytgrab\\${j.title || j.id}.${j.preset.startsWith('audio') ? j.preset.slice(6) : 'mp4'}`;
        changed = true;
      }
    }
    const running = this.jobs.filter((j) =>
      ['inspecting', 'downloading', 'processing'].includes(j.state),
    ).length;
    const next = this.jobs.filter((j) => j.state === 'queued').at(-1);
    if (next && running < 2) {
      next.state = 'downloading';
      next.progress = {
        downloaded_bytes: 0,
        total_bytes: 40_000_000,
        speed_bps: 3_000_000,
        eta_seconds: null,
      };
      changed = true;
    }
    if (changed) this.emit();
  }

  emit() {
    const snapshot = structuredClone(this.jobs);
    for (const fn of this.listeners) fn(snapshot);
  }
}

class HttpClient {
  isFixture = false;

  // { version, latest, update_available, update_command, release_url }
  version() {
    return this.request('GET', '/api/system/version');
  }
  health() {
    return this.request('GET', '/api/system/health');
  }
  listJobs() {
    return this.request('GET', '/api/jobs').then((r) => {
      this.pausedUntil = r.paused_until ?? null;
      return r.jobs ?? r;
    });
  }
  // Opens the system file manager at a finished download.
  revealJob(id) {
    return this.request('POST', `/api/jobs/${encodeURIComponent(id)}/reveal`);
  }
  // Removes a finished job from the history; with deleteFile, also deletes its file.
  deleteJob(id, deleteFile = false) {
    const query = deleteFile ? '?delete_file=true' : '';
    return this.request('DELETE', `/api/jobs/${encodeURIComponent(id)}${query}`).then(this.afterChange);
  }
  // Removes every finished, failed, and cancelled job; files are kept.
  clearHistory() {
    return this.request('POST', '/api/history/clear').then((result) => this.afterChange(result));
  }
  // Installs or updates yt-dlp on the server; resolves with { version, previous, updated }.
  updateYtdlp() {
    return this.request('POST', '/api/system/update-ytdlp');
  }
  resume() {
    return this.request('POST', '/api/system/resume').then(() => {
      this.pausedUntil = null;
      this.refreshNow?.();
    });
  }
  createJob(body) {
    return this.request('POST', '/api/jobs', body).then(this.afterChange);
  }
  inspect(url, { signal } = {}) {
    return this.request('GET', `/api/inspect?url=${encodeURIComponent(url)}`, undefined, signal);
  }
  listPlaylist(url, { signal, start = 1 } = {}) {
    return this.request('GET', `/api/playlist?url=${encodeURIComponent(url)}&start=${start}`, undefined, signal);
  }
  createPlaylistJobs(body) {
    return this.request('POST', '/api/playlist/jobs', body).then(this.afterChange);
  }
  cancelJob(id) {
    return this.request('POST', `/api/jobs/${encodeURIComponent(id)}/cancel`).then(this.afterChange);
  }
  retryJob(id) {
    return this.request('POST', `/api/jobs/${encodeURIComponent(id)}/retry`).then(this.afterChange);
  }

  // A change made from this page refreshes the subscription right away, so a new job's
  // progress stream opens without waiting for the idle poll interval.
  afterChange = (result) => {
    this.refreshNow?.();
    return result;
  };
  getSettings() {
    return this.request('GET', '/api/settings');
  }
  updateSettings(body) {
    return this.request('PUT', '/api/settings', body);
  }
  // Saves any of { max_downloads, default_preset, speed_limit_kbps }.
  setPreferences(body) {
    return this.request('PUT', '/api/settings/preferences', body);
  }
  // Subtitles for video downloads: mode is 'off', 'embed', or 'file'; lang is a listed code.
  setSubtitles({ mode, lang }) {
    return this.request('PUT', '/api/settings/subtitles', { mode, lang });
  }
  // Starts YTGrab in the tray when the user signs in to Windows, or stops doing so.
  setStartAtLogin(enabled) {
    return this.request('PUT', '/api/settings/startup', { enabled });
  }
  // Turns browser sign-in on for one of the listed browsers, or off with ''.
  setCookiesBrowser(browser) {
    return this.request('PUT', '/api/settings/cookies', { browser });
  }
  // Opens the operating system's folder window on this computer; resolves when it closes.
  pickFolder() {
    return this.request('POST', '/api/settings/pick-folder');
  }
  useDefaultFolder() {
    return this.request('POST', '/api/settings/use-default');
  }

  // Poll for queue/history changes; stream persisted progress for active jobs.
  subscribe(onChange) {
    let timer;
    let closed = false;
    let jobs = [];
    const sources = new Map();
    const syncSources = () => {
      if (typeof EventSource === 'undefined') return;
      const running = new Set(jobs.filter((job) =>
        ['inspecting', 'downloading', 'processing'].includes(job.state)).map((job) => job.id));
      for (const [id, source] of sources) {
        if (!running.has(id)) (source.close(), sources.delete(id));
      }
      for (const id of running) {
        // Each stream holds a connection; browsers allow about six per site.
        if (sources.has(id) || sources.size >= MAX_STREAMS) continue;
        const source = new EventSource(`/api/jobs/${encodeURIComponent(id)}/events`);
        sources.set(id, source);
        source.onmessage = (event) => {
          if (closed) return;
          try {
            const job = JSON.parse(event.data);
            jobs = jobs.map((current) => current.id === job.id ? job : current);
            onChange([...jobs]);
            if (!isActive(job.state)) (source.close(), sources.delete(id));
          } catch { /* The next poll refreshes the snapshot. */ }
        };
        source.onerror = () => (source.close(), sources.delete(id));
      }
    };
    // One poll chain at a time: a refresh requested mid-poll runs when that poll ends.
    let polling = false;
    let again = false;
    const schedule = (ms) => {
      clearTimeout(timer);
      if (!closed) timer = setTimeout(poll, again ? 0 : ms);
      again = false;
    };
    const poll = () => {
      if (polling) return void (again = true);
      polling = true;
      this.listJobs().then(
        (snapshot) => {
          polling = false;
          if (closed) return;
          jobs = snapshot;
          onChange([...jobs]);
          syncSources();
          // Refresh quickly while work is queued or running; slowly when idle.
          schedule(jobs.some((job) => isActive(job.state)) ? 2000 : 10000);
        },
        (err) => {
          polling = false;
          if (err?.code !== 'not_available') schedule(2000);
        },
      );
    };
    timer = setTimeout(poll, 2000);
    this.refreshNow = () => {
      if (closed) return;
      clearTimeout(timer);
      poll();
    };
    return () => {
      closed = true;
      this.refreshNow = null;
      clearTimeout(timer);
      for (const source of sources.values()) source.close();
      sources.clear();
    };
  }

  async request(method, path, body, signal) {
    let res;
    try {
      res = await fetch(path, {
        method,
        signal,
        headers: body ? { 'Content-Type': 'application/json' } : undefined,
        body: body ? JSON.stringify(body) : undefined,
      });
    } catch (err) {
      if (err.name === 'AbortError') throw err;
      throw new ApiError(
        'unreachable',
        'Could not reach the local server. Is ytgrab still running?',
      );
    }
    const data = await res.json().catch(() => null);
    if (!res.ok && !data?.error && (res.status === 404 || res.status === 405)) {
      throw new ApiError(
        'not_available',
        "This version of the ytgrab server can't manage downloads yet.",
      );
    }
    if (!res.ok) {
      throw new ApiError(
        data?.error?.code || `http_${res.status}`,
        data?.error?.message || `Request failed (${res.status}).`,
      );
    }
    return data;
  }
}

const MAX_STREAMS = 4;

export const isActive = (state) =>
  ['queued', 'inspecting', 'downloading', 'processing'].includes(state);

const delay = (ms, signal) =>
  new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal?.addEventListener('abort', () => {
      clearTimeout(timer);
      reject(new DOMException('Aborted', 'AbortError'));
    });
  });

// The server owns real labels; the fixture derives one from the sample inspection.
function fixtureFormatLabel({ kind, id }) {
  const list = kind === 'video' ? fx.inspection.video : fx.inspection.audio;
  const f = list.find((x) => x.format_id === id);
  if (!f) return `Format ${id}`;
  return kind === 'video' ? `Video · ${f.height}p` : `Audio · ${f.ext.toUpperCase()} ${Math.round(f.abr)} kbps`;
}
