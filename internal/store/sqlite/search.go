package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

// SearchChunk is a piece of a download's text: its title (Start -1) or a few subtitle lines
// starting at Start seconds.
type SearchChunk struct {
	Start float64
	Text  string
}

// SearchHit is one matching chunk, best first; Snippet marks the matched words with
// \x02 and \x03.
type SearchHit struct {
	JobID   string
	Start   float64
	Snippet string
}

// CompletedJobs returns every finished download, for the search index.
func (store *Store) CompletedJobs(ctx context.Context) ([]domain.Job, error) {
	rows, err := store.db.QueryContext(ctx, "SELECT payload FROM jobs WHERE state='completed'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []domain.Job
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var job domain.Job
		if err := json.Unmarshal(payload, &job); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// IndexedStamps returns what was indexed for each download.
func (store *Store) IndexedStamps(ctx context.Context) (map[string]string, error) {
	rows, err := store.db.QueryContext(ctx, "SELECT job_id, stamp FROM search_indexed")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stamps := map[string]string{}
	for rows.Next() {
		var id, stamp string
		if err := rows.Scan(&id, &stamp); err != nil {
			return nil, err
		}
		stamps[id] = stamp
	}
	return stamps, rows.Err()
}

// IndexJob replaces a download's text in the search index.
func (store *Store) IndexJob(ctx context.Context, jobID, stamp string, chunks []SearchChunk) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := unindex(ctx, tx, jobID); err != nil {
		return err
	}
	for _, chunk := range chunks {
		if _, err := tx.ExecContext(ctx, "INSERT INTO search(job_id, start, text) VALUES(?, ?, ?)", jobID, chunk.Start, chunk.Text); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO search_indexed(job_id, stamp) VALUES(?, ?)", jobID, stamp); err != nil {
		return err
	}
	return tx.Commit()
}

// Unindex removes a download from the search index.
func (store *Store) Unindex(ctx context.Context, jobID string) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := unindex(ctx, tx, jobID); err != nil {
		return err
	}
	return tx.Commit()
}

func unindex(ctx context.Context, tx *sql.Tx, jobID string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM search WHERE job_id=?", jobID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM search_indexed WHERE job_id=?", jobID)
	return err
}

// Search finds chunks matching an FTS5 query, best first.
func (store *Store) Search(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	rows, err := store.db.QueryContext(ctx,
		"SELECT job_id, start, snippet(search, 2, char(2), char(3), '…', 14) FROM search WHERE search MATCH ? ORDER BY rank LIMIT ?", query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var hit SearchHit
		if err := rows.Scan(&hit.JobID, &hit.Start, &hit.Snippet); err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}
