package sqlitestore

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrNotFound = errors.New("job not found")
var ErrConflict = errors.New("job changed while updating")
var ErrDuplicate = errors.New("this video is already queued")

type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON"} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure sqlite: %w", err)
		}
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (store *Store) Close() error { return store.db.Close() }

func (store *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := store.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return value, err == nil, err
}

func (store *Store) PutSetting(ctx context.Context, key, value string) error {
	_, err := store.db.ExecContext(ctx,
		"INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return err
}

func migrate(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		name := filepath.Base(file)
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("invalid migration name %q: %w", name, err)
		}
		if version <= current {
			continue
		}
		contents, err := migrations.ReadFile(file)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, statement := range strings.Split(string(contents), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply migration %s: %w", name, err)
			}
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", version)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		current = version
	}
	return nil
}

func (store *Store) Create(ctx context.Context, job domain.Job) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	result, err := store.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO jobs(id, video_id, state, created_at, updated_at, payload) VALUES(?, ?, ?, ?, ?, ?)",
		job.ID, job.VideoID, job.State, job.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), job.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), payload)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrDuplicate
	}
	return nil
}

func (store *Store) Get(ctx context.Context, id string) (domain.Job, error) {
	var payload []byte
	err := store.db.QueryRowContext(ctx, "SELECT payload FROM jobs WHERE id=?", id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, ErrNotFound
	}
	if err != nil {
		return domain.Job{}, err
	}
	var job domain.Job
	return job, json.Unmarshal(payload, &job)
}

func (store *Store) List(ctx context.Context, limit int) ([]domain.Job, error) {
	return store.list(ctx, "SELECT payload FROM jobs ORDER BY created_at DESC LIMIT ?", limit)
}

func (store *Store) Queued(ctx context.Context, limit int) ([]domain.Job, error) {
	return store.list(ctx, "SELECT payload FROM jobs WHERE state='queued' ORDER BY created_at LIMIT ?", limit)
}

func (store *Store) list(ctx context.Context, query string, limit int) ([]domain.Job, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	rows, err := store.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]domain.Job, 0)
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

// Update atomically saves a job if its database state still matches expected.
func (store *Store) Update(ctx context.Context, job domain.Job, expected domain.State) error {
	if !domain.CanTransition(expected, job.State) {
		return fmt.Errorf("invalid transition from %s to %s", expected, job.State)
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	result, err := store.db.ExecContext(ctx, "UPDATE jobs SET state=?, updated_at=?, payload=? WHERE id=? AND state=?",
		job.State, job.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), payload, job.ID, expected)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrConflict
	}
	return nil
}

// Recover marks work left active by a previous process as retryable failures.
func (store *Store) Recover(ctx context.Context) (int, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT payload FROM jobs WHERE state IN ('inspecting','downloading','processing')")
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	var jobs []domain.Job
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			_ = rows.Close()
			_ = tx.Rollback()
			return 0, err
		}
		var job domain.Job
		if err := json.Unmarshal(payload, &job); err != nil {
			_ = rows.Close()
			_ = tx.Rollback()
			return 0, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		_ = tx.Rollback()
		return 0, err
	}
	_ = rows.Close()
	for _, job := range jobs {
		job.State = domain.Failed
		job.Error = &domain.JobError{Code: "interrupted", Message: "The app closed while this download was running. Retry to resume from the partial file."}
		job.UpdatedAt = time.Now().UTC()
		payload, err := json.Marshal(job)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE jobs SET state=?, updated_at=?, payload=? WHERE id=?",
			job.State, job.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), payload, job.ID); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(jobs), nil
}
