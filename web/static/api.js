// Data clients. The UI talks only to this interface:
//   health(), listJobs(), createJob({url, preset}), cancelJob(id), retryJob(id), subscribe(onChange),
//   getSettings(), updateSettings({downloads_dir})
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

// Scenarios: default | empty | error | degraded | loading
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

  async health() {
    await delay(150);
    if (this.scenario === 'error')
      throw new ApiError('unreachable', 'Could not reach the local server.');
    return this.scenario === 'degraded' ? fx.healthDegraded : fx.healthOk;
  }

  async listJobs() {
    await delay(this.scenario === 'loading' ? 1e9 : 300);
    if (this.scenario === 'error') {
      throw new ApiError(
        'unreachable',
        'Could not reach the local server. Is ytgrab still running?',
      );
    }
    return structuredClone(this.jobs);
  }

  async createJob({ url, preset }) {
    await delay(400);
    if (this.scenario === 'error')
      throw new ApiError('unreachable', 'Could not reach the local server.');
    if (this.jobs.some((j) => j.url === url && isActive(j.state))) {
      throw new ApiError('duplicate_job', 'This link is already in the queue.');
    }
    const at = new Date().toISOString();
    const job = {
      id: `job_${this.nextId++}`,
      url,
      video_id: null,
      title: null,
      preset,
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

  async getSettings() {
    await delay(200);
    if (this.scenario === 'error')
      throw new ApiError('unreachable', 'Could not reach the local server.');
    return { downloads_dir: this.downloadsDir };
  }

  async updateSettings({ downloads_dir }) {
    await delay(300);
    // Loosely mirrors the server rule: an absolute path to an existing, writable folder.
    if (!/^([a-z]:[\\/]|\\\\|\/)/i.test(downloads_dir)) {
      throw new ApiError('invalid_directory', String.raw`Use a full folder path, such as D:\Videos.`);
    }
    this.downloadsDir = downloads_dir;
    return { downloads_dir };
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

  health() {
    return this.request('GET', '/api/system/health');
  }
  listJobs() {
    return this.request('GET', '/api/jobs').then((r) => r.jobs ?? r);
  }
  createJob(body) {
    return this.request('POST', '/api/jobs', body);
  }
  cancelJob(id) {
    return this.request('POST', `/api/jobs/${encodeURIComponent(id)}/cancel`);
  }
  retryJob(id) {
    return this.request('POST', `/api/jobs/${encodeURIComponent(id)}/retry`);
  }
  getSettings() {
    return this.request('GET', '/api/settings');
  }
  updateSettings(body) {
    return this.request('PUT', '/api/settings', body);
  }

  // Per-job SSE lands with the download engine; poll the list until then.
  // Polling stops if the server has no job routes, instead of repeating 404s.
  subscribe(onChange) {
    let timer;
    const poll = () =>
      this.listJobs()
        .then(onChange, (err) => err)
        .then((err) => {
          if (err?.code !== 'not_available') timer = setTimeout(poll, 2000);
        });
    timer = setTimeout(poll, 2000);
    return () => clearTimeout(timer);
  }

  async request(method, path, body) {
    let res;
    try {
      res = await fetch(path, {
        method,
        headers: body ? { 'Content-Type': 'application/json' } : undefined,
        body: body ? JSON.stringify(body) : undefined,
      });
    } catch {
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

export const isActive = (state) =>
  ['queued', 'inspecting', 'downloading', 'processing'].includes(state);

const delay = (ms) => new Promise((r) => setTimeout(r, ms));
