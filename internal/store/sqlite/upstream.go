package sqlitestore

import (
	"context"
	"database/sql"
	"time"
)

// UpstreamVideo is a downloaded video to check online.
type UpstreamVideo struct {
	Site    string // "" (YouTube) or "vimeo"
	VideoID string
	Title   string
}

// UpstreamDue lists finished YouTube and Vimeo downloads not checked since before, the
// least recently checked (or never checked) first.
func (store *Store) UpstreamDue(ctx context.Context, before time.Time, limit int) ([]UpstreamVideo, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT j.site, j.video_id, MAX(j.title) FROM (
			SELECT COALESCE(json_extract(payload, '$.site'), '') AS site, video_id, COALESCE(json_extract(payload, '$.title'), '') AS title
			FROM jobs WHERE state = 'completed'
		) j
		LEFT JOIN upstream u ON u.site = j.site AND u.video_id = j.video_id
		WHERE j.site IN ('', 'vimeo') AND (u.checked_at IS NULL OR u.checked_at < ?)
		GROUP BY j.site, j.video_id
		ORDER BY MIN(COALESCE(u.checked_at, '')) LIMIT ?`, stamp(before), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var videos []UpstreamVideo
	for rows.Next() {
		var video UpstreamVideo
		if err := rows.Scan(&video.Site, &video.VideoID, &video.Title); err != nil {
			return nil, err
		}
		videos = append(videos, video)
	}
	return videos, rows.Err()
}

// SetUpstream records a check: gone or online. It reports whether the video has just gone
// (it was online, or never checked, before).
func (store *Store) SetUpstream(ctx context.Context, site, videoID string, gone bool, at time.Time) (bool, error) {
	var goneAt sql.NullString
	err := store.db.QueryRowContext(ctx, "SELECT gone_at FROM upstream WHERE site=? AND video_id=?", site, videoID).Scan(&goneAt)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	wasGone := goneAt.Valid
	next := sql.NullString{}
	if gone {
		next = sql.NullString{String: stamp(at), Valid: true}
		if wasGone {
			next = goneAt // keep when it was first found gone
		}
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO upstream(site, video_id, checked_at, gone_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(site, video_id) DO UPDATE SET checked_at=excluded.checked_at, gone_at=excluded.gone_at`, site, videoID, stamp(at), next)
	return gone && !wasGone, err
}

// GoneSince maps each gone video ("<site>:<id>") to when it was found gone.
func (store *Store) GoneSince(ctx context.Context) (map[string]time.Time, error) {
	rows, err := store.db.QueryContext(ctx, "SELECT site, video_id, gone_at FROM upstream WHERE gone_at IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	gone := map[string]time.Time{}
	for rows.Next() {
		var site, id, at string
		if err := rows.Scan(&site, &id, &at); err != nil {
			return nil, err
		}
		if when, err := time.Parse(time.RFC3339Nano, at); err == nil {
			gone[site+":"+id] = when
		}
	}
	return gone, rows.Err()
}

func stamp(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }
