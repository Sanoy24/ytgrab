-- Whether downloaded videos are still online: when each was last checked, and since when
-- it has been gone (deleted or removed by the site).
CREATE TABLE upstream (
    site TEXT NOT NULL,
    video_id TEXT NOT NULL,
    checked_at TEXT NOT NULL,
    gone_at TEXT,
    PRIMARY KEY (site, video_id)
) WITHOUT ROWID;
