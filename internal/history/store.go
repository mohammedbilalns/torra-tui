package history

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mohammedbilalns/torra-cli/internal/models"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) init() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS torrents (
			id TEXT PRIMARY KEY,
			magnet TEXT NOT NULL,
			name TEXT NOT NULL,
			download_dir TEXT NOT NULL,
			state TEXT NOT NULL,
			completed INTEGER NOT NULL,
			bytes_completed INTEGER NOT NULL,
			length INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);
	`)
	return err
}

func (s *Store) List() ([]models.TorrentEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, magnet, name, download_dir, state, completed, bytes_completed, length, created_at
		FROM torrents
		ORDER BY created_at DESC;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.TorrentEntry
	for rows.Next() {
		var e models.TorrentEntry
		var created string
		var completed int
		if err := rows.Scan(&e.ID, &e.Magnet, &e.Name, &e.DownloadDir, &e.State, &completed, &e.BytesCompleted, &e.Length, &created); err != nil {
			return nil, err
		}
		e.Completed = completed == 1
		if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
			e.CreatedAt = t
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) Upsert(e models.TorrentEntry) error {
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	completed := 0
	if e.Completed {
		completed = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO torrents (id, magnet, name, download_dir, state, completed, bytes_completed, length, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			magnet=excluded.magnet,
			name=excluded.name,
			download_dir=excluded.download_dir,
			state=excluded.state,
			completed=excluded.completed,
			bytes_completed=excluded.bytes_completed,
			length=excluded.length;
	`, e.ID, e.Magnet, e.Name, e.DownloadDir, e.State, completed, e.BytesCompleted, e.Length, created.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("upsert torrent: %w", err)
	}
	return nil
}

func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM torrents WHERE id = ?;`, id)
	return err
}
