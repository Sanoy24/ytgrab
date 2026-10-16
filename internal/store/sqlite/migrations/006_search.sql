-- Library search: each finished download's title (start -1) and its subtitles in short
-- chunks, with where each chunk starts (seconds).
CREATE VIRTUAL TABLE search USING fts5(job_id UNINDEXED, start UNINDEXED, text, tokenize='unicode61 remove_diacritics 2');
-- What was indexed for each download, so changed files are indexed again.
CREATE TABLE search_indexed (
    job_id TEXT PRIMARY KEY,
    stamp TEXT NOT NULL
) WITHOUT ROWID;
