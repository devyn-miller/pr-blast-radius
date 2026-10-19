package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
)

type Store interface {
	GetFileCaptures(filePath, contentSHA string) (*astparse.FileCaptures, bool, error)
	PutFileCaptures(filePath, contentSHA string, fc astparse.FileCaptures) error
	Stats() (hits int, misses int)
	Close() error
}

type SQLiteStore struct {
	db     *sql.DB
	hits   int
	misses int
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("cache: opening sqlite db at %s: %w", dbPath, err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS file_captures (
			file_path TEXT NOT NULL,
			content_sha TEXT NOT NULL,
			captures_json BLOB NOT NULL,
			cached_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (file_path, content_sha)
		);
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: creating schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) GetFileCaptures(filePath, contentSHA string) (*astparse.FileCaptures, bool, error) {
	row := s.db.QueryRow(
		`SELECT captures_json FROM file_captures WHERE file_path = ? AND content_sha = ?`,
		filePath, contentSHA,
	)

	var blob []byte
	err := row.Scan(&blob)
	if err == sql.ErrNoRows {
		s.misses++
		return nil, false, nil // a miss is normal/expected, not an error
	}
	if err != nil {
		return nil, false, fmt.Errorf("cache: querying %s: %w", filePath, err)
	}

	var fc astparse.FileCaptures
	if err := json.Unmarshal(blob, &fc); err != nil {
		return nil, false, fmt.Errorf("cache: corrupt cache entry for %s: %w", filePath, err)
	}

	s.hits++
	return &fc, true, nil
}

func (s *SQLiteStore) PutFileCaptures(filePath, contentSHA string, fc astparse.FileCaptures) error {
	blob, err := json.Marshal(fc)
	if err != nil {
		return fmt.Errorf("cache: marshaling captures for %s: %w", filePath, err)
	}

	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO file_captures (file_path, content_sha, captures_json) VALUES (?, ?, ?)`,
		filePath, contentSHA, blob,
	)
	return err
}

func (s *SQLiteStore) Stats() (int, int) { return s.hits, s.misses }
func (s *SQLiteStore) Close() error      { return s.db.Close() }
