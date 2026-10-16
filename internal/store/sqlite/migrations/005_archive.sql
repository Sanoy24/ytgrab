-- Videos the user already has from another tool, imported from a yt-dlp download archive.
CREATE TABLE archive (
    site TEXT NOT NULL,
    video_id TEXT NOT NULL,
    PRIMARY KEY (site, video_id)
) WITHOUT ROWID;
