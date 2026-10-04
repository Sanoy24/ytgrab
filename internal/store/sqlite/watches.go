package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

var ErrWatchExists = errors.New("YTGrab is already watching this channel or playlist.")

// CreateWatch saves a new watch and marks seen as its already-known videos, together.
func (store *Store) CreateWatch(ctx context.Context, watch domain.Watch, seen []string) error {
	payload, err := json.Marshal(watch)
	if err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO watches(id, url, created_at, payload) VALUES(?, ?, ?, ?)",
		watch.ID, watch.URL, watch.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), payload)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrWatchExists
	}
	if err := markSeen(ctx, tx, watch.ID, seen); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *Store) Watches(ctx context.Context) ([]domain.Watch, error) {
	rows, err := store.db.QueryContext(ctx, "SELECT payload FROM watches ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	watches := []domain.Watch{}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var watch domain.Watch
		if err := json.Unmarshal(payload, &watch); err != nil {
			return nil, err
		}
		watches = append(watches, watch)
	}
	return watches, rows.Err()
}

func (store *Store) GetWatch(ctx context.Context, id string) (domain.Watch, error) {
	var payload []byte
	err := store.db.QueryRowContext(ctx, "SELECT payload FROM watches WHERE id=?", id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Watch{}, ErrNotFound
	}
	if err != nil {
		return domain.Watch{}, err
	}
	var watch domain.Watch
	return watch, json.Unmarshal(payload, &watch)
}

func (store *Store) UpdateWatch(ctx context.Context, watch domain.Watch) error {
	payload, err := json.Marshal(watch)
	if err != nil {
		return err
	}
	result, err := store.db.ExecContext(ctx, "UPDATE watches SET payload=? WHERE id=?", payload, watch.ID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteWatch removes a watch and what it has seen; its downloads stay in the Library.
func (store *Store) DeleteWatch(ctx context.Context, id string) error {
	result, err := store.db.ExecContext(ctx, "DELETE FROM watches WHERE id=?", id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// Unseen returns the given video IDs the watch hasn't seen, in the same order.
func (store *Store) Unseen(ctx context.Context, watchID string, videoIDs []string) ([]string, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	query := "SELECT video_id FROM watch_seen WHERE watch_id=? AND video_id IN (?" + strings.Repeat(",?", len(videoIDs)-1) + ")"
	args := []any{watchID}
	for _, id := range videoIDs {
		args = append(args, id)
	}
	rows, err := store.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		seen[id] = true
	}
	unseen := []string{}
	for _, id := range videoIDs {
		if !seen[id] {
			unseen = append(unseen, id)
		}
	}
	return unseen, rows.Err()
}

func (store *Store) MarkSeen(ctx context.Context, watchID string, videoIDs []string) error {
	return markSeen(ctx, store.db, watchID, videoIDs)
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func markSeen(ctx context.Context, db execer, watchID string, videoIDs []string) error {
	for _, id := range videoIDs {
		if _, err := db.ExecContext(ctx, "INSERT OR IGNORE INTO watch_seen(watch_id, video_id) VALUES(?, ?)", watchID, id); err != nil {
			return err
		}
	}
	return nil
}

// SeenIDs returns every video ID the watch has seen, for a backup.
func (store *Store) SeenIDs(ctx context.Context, watchID string) ([]string, error) {
	rows, err := store.db.QueryContext(ctx, "SELECT video_id FROM watch_seen WHERE watch_id=? ORDER BY video_id", watchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
