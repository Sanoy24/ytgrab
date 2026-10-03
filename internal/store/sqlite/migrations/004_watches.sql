-- Channels and playlists YTGrab follows, and the videos each has already seen.
CREATE TABLE watches (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    payload TEXT NOT NULL
);
CREATE TABLE watch_seen (
    watch_id TEXT NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
    video_id TEXT NOT NULL,
    PRIMARY KEY (watch_id, video_id)
) WITHOUT ROWID;
