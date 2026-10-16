package sqlitestore

import (
	"context"
	"strings"
)

// ArchivedVideo is one video of an imported yt-dlp download archive.
type ArchivedVideo struct {
	Site    string // domain.SiteYouTube ("") or domain.SiteVimeo
	VideoID string
}

// AddArchived remembers videos the user already has; it reports how many were new.
func (store *Store) AddArchived(ctx context.Context, videos []ArchivedVideo) (int, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, video := range videos {
		result, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO archive(site, video_id) VALUES(?, ?)", video.Site, video.VideoID)
		if err != nil {
			return 0, err
		}
		if rows, _ := result.RowsAffected(); rows > 0 {
			added++
		}
	}
	return added, tx.Commit()
}

// Have reports which of the video IDs the user already has: finished in the Library, or
// in an imported archive.
func (store *Store) Have(ctx context.Context, ids []string) (map[string]bool, error) {
	have := map[string]bool{}
	for start := 0; start < len(ids); start += 400 {
		batch := ids[start:min(start+400, len(ids))]
		marks := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, 0, 2*len(batch))
		for _, id := range batch {
			args = append(args, id)
		}
		for _, id := range batch {
			args = append(args, id)
		}
		rows, err := store.db.QueryContext(ctx,
			"SELECT video_id FROM archive WHERE video_id IN ("+marks+") UNION SELECT video_id FROM jobs WHERE state='completed' AND video_id IN ("+marks+")", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			have[id] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return have, nil
}
