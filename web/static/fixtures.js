// Sample data for building the UI before the backend API exists.
// Job shapes follow the proposal in web/README.md.

const now = Date.now();
const ago = (minutes) => new Date(now - minutes * 60_000).toISOString();

// Health fixtures mirror real /api/system/health responses from internal/app/deps.
const note = "Executable checks only. YouTube access and yt-dlp-ejs availability are not verified.";
const tool = (name, version, path) => ({ name, available: true, required: true, path, version, message: "Available." });

export const healthOk = {
  status: "ready",
  checked_at: ago(0),
  dependencies: [
    tool("yt-dlp", "2026.09.10", String.raw`C:\ytgrab\tools\yt-dlp.exe`),
    tool("ffmpeg", "ffmpeg version 8.0.1-full_build-www.gyan.dev Copyright (c) 2000-2025 the FFmpeg developers", String.raw`C:\ytgrab\tools\ffmpeg.exe`),
    tool("ffprobe", "ffprobe version 8.0.1-full_build-www.gyan.dev Copyright (c) 2007-2025 the FFmpeg developers", String.raw`C:\ytgrab\tools\ffprobe.exe`),
    { ...tool("js-runtime", "v24.4.1", String.raw`C:\Program Files\nodejs\node.exe`), message: "Node is available; yt-dlp must be run with --js-runtimes node." },
  ],
  note,
};

// Captured from the Go server on a machine without yt-dlp (paths shortened).
export const healthDegraded = {
  status: "degraded",
  checked_at: ago(0),
  dependencies: [
    { name: "yt-dlp", available: false, required: true, message: "Install yt-dlp and add it to PATH or the tools directory." },
    { name: "ffmpeg", available: false, required: true, message: "Install ffmpeg and add it to PATH or the tools directory." },
    healthOk.dependencies[2],
    healthOk.dependencies[3],
  ],
  note,
};

export const jobs = [
  {
    id: "job_7",
    url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
    video_id: "dQw4w9WgXcQ",
    title: "Building a local-first download manager in Go — full walkthrough",
    preset: "video-1080",
    state: "downloading",
    attempt: 1,
    progress: { downloaded_bytes: 187_000_000, total_bytes: 412_000_000, speed_bps: 6_400_000, eta_seconds: 35 },
    output_path: null,
    error: null,
    created_at: ago(2),
    updated_at: ago(0),
  },
  {
    id: "job_6",
    url: "https://youtu.be/aqz-KE-bpKQ",
    video_id: "aqz-KE-bpKQ",
    title: null,
    preset: "audio-m4a",
    state: "downloading",
    attempt: 1,
    // Live streams and some formats report no total size or ETA.
    progress: { downloaded_bytes: 9_800_000, total_bytes: null, speed_bps: 1_200_000, eta_seconds: null },
    output_path: null,
    error: null,
    created_at: ago(3),
    updated_at: ago(0),
  },
  {
    id: "job_8",
    url: "https://www.youtube.com/watch?v=jNQXAC9IVRw",
    video_id: "jNQXAC9IVRw",
    title: "Me at the zoo",
    preset: "audio-mp3",
    state: "queued",
    attempt: 1,
    progress: null,
    output_path: null,
    error: null,
    created_at: ago(1),
    updated_at: ago(1),
  },
  {
    id: "job_5",
    url: "https://www.youtube.com/watch?v=M7lc1UVf-VE",
    video_id: "M7lc1UVf-VE",
    title: "YouTube Developers Live: Embedded Web Player Customization",
    preset: "video-best",
    state: "completed",
    attempt: 1,
    progress: { downloaded_bytes: 248_000_000, total_bytes: 248_000_000, speed_bps: null, eta_seconds: null },
    output_path: "C:\\Users\\me\\Downloads\\ytgrab\\YouTube Developers Live [M7lc1UVf-VE].mp4",
    error: null,
    created_at: ago(55),
    updated_at: ago(51),
  },
  {
    id: "job_4",
    url: "https://www.youtube.com/watch?v=xxxxxxxxxxx",
    video_id: "xxxxxxxxxxx",
    title: null,
    preset: "video-best",
    state: "failed",
    attempt: 2,
    progress: null,
    output_path: null,
    error: { code: "video_unavailable", message: "This video is private or has been removed." },
    created_at: ago(90),
    updated_at: ago(88),
  },
  {
    id: "job_3",
    url: "https://www.youtube.com/watch?v=9bZkp7q19f0",
    video_id: "9bZkp7q19f0",
    title: "Long concert recording (4K)",
    preset: "video-best",
    state: "failed",
    attempt: 1,
    progress: { downloaded_bytes: 1_300_000_000, total_bytes: 3_100_000_000, speed_bps: null, eta_seconds: null },
    output_path: null,
    error: {
      code: "interrupted",
      message: "The app closed while this download was running. Retry to resume from the partial file.",
    },
    created_at: ago(60 * 20),
    updated_at: ago(60 * 19),
  },
  {
    id: "job_2",
    url: "https://www.youtube.com/watch?v=kJQP7kiw5Fk",
    video_id: "kJQP7kiw5Fk",
    title: "Podcast episode 112",
    preset: "audio-m4a",
    state: "cancelled",
    attempt: 1,
    progress: null,
    output_path: null,
    error: null,
    created_at: ago(60 * 26),
    updated_at: ago(60 * 26),
  },
  {
    id: "job_1",
    url: "https://www.youtube.com/watch?v=YE7VzlLtp-4",
    video_id: "YE7VzlLtp-4",
    title: "Lecture 1 — Introduction",
    preset: "audio-mp3",
    state: "completed",
    attempt: 1,
    progress: { downloaded_bytes: 61_000_000, total_bytes: 61_000_000, speed_bps: null, eta_seconds: null },
    output_path: "C:\\Users\\me\\Downloads\\ytgrab\\Lecture 1 — Introduction [YE7VzlLtp-4].mp3",
    error: null,
    created_at: ago(60 * 50),
    updated_at: ago(60 * 49),
  },
];
