CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    video_id TEXT NOT NULL,
    state TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    payload TEXT NOT NULL
);
CREATE INDEX jobs_created_at ON jobs(created_at DESC);
CREATE INDEX jobs_state_created_at ON jobs(state, created_at);
CREATE UNIQUE INDEX jobs_active_video ON jobs(video_id)
    WHERE state IN ('queued', 'inspecting', 'downloading', 'processing');
