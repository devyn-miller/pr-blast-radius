package cache

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
)

const pyPath = "services/pricing.py"

var (
	pySource        = []byte("def price(x):\n    return x * 2\n")
	pySourceChanged = []byte("def price(x, currency):\n    return x * 2\n")
)

// newTestStore uses a file-backed DB: with database/sql's connection pool,
// ":memory:" can hand each connection its own empty database.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "cache.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func newTestRegistry(t *testing.T) *astparse.Registry {
	t.Helper()
	goParser, err := astparse.NewGoParser()
	require.NoError(t, err)
	pyParser, err := astparse.NewPythonParser()
	require.NoError(t, err)
	return astparse.NewRegistry(goParser, pyParser)
}

func TestCachedRegistry_WarmRun_HitsCache(t *testing.T) {
	store := newTestStore(t)
	reg := NewCachedRegistry(newTestRegistry(t), store)
	ctx := context.Background()

	cold, err := reg.ParseFile(ctx, pyPath, pySource)
	require.NoError(t, err)
	hits, misses := store.Stats()
	assert.Equal(t, 0, hits)
	assert.Equal(t, 1, misses)

	warm, err := reg.ParseFile(ctx, pyPath, pySource)
	require.NoError(t, err)
	hits, misses = store.Stats()
	assert.Equal(t, 1, hits, "second parse of identical content should be served from cache")
	assert.Equal(t, 1, misses)

	assert.Equal(t, cold, warm, "cached captures must round-trip unchanged")
	require.NotEmpty(t, warm.Symbols)
}

func TestCachedRegistry_ChangedContent_MissesCache(t *testing.T) {
	store := newTestStore(t)
	reg := NewCachedRegistry(newTestRegistry(t), store)
	ctx := context.Background()

	original, err := reg.ParseFile(ctx, pyPath, pySource)
	require.NoError(t, err)

	changed, err := reg.ParseFile(ctx, pyPath, pySourceChanged)
	require.NoError(t, err)

	hits, misses := store.Stats()
	assert.Equal(t, 0, hits, "same path with new content must not reuse the old entry")
	assert.Equal(t, 2, misses)
	assert.NotEqual(t, original.ContentSHA, changed.ContentSHA)
}

func TestSQLiteStore_CorruptEntry_ReturnsError(t *testing.T) {
	store := newTestStore(t)
	sha := contentHash(pySource)
	require.NoError(t, store.PutFileCaptures(pyPath, sha, astparse.FileCaptures{FilePath: pyPath}))

	_, err := store.db.Exec(
		`UPDATE file_captures SET captures_json = ? WHERE file_path = ? AND content_sha = ?`,
		[]byte("{not json"), pyPath, sha,
	)
	require.NoError(t, err)

	fc, hit, err := store.GetFileCaptures(pyPath, sha)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "corrupt cache entry")
	assert.False(t, hit)
	assert.Nil(t, fc)
}

func TestCachedRegistry_CorruptEntry_FallsBackToParseAndRepairs(t *testing.T) {
	// CachedRegistry deliberately degrades on store errors instead of failing
	// the whole graph build: it re-parses, and the re-parse overwrites the
	// bad row so the next run is a clean hit.
	store := newTestStore(t)
	reg := NewCachedRegistry(newTestRegistry(t), store)
	ctx := context.Background()
	sha := contentHash(pySource)

	want, err := reg.ParseFile(ctx, pyPath, pySource)
	require.NoError(t, err)

	_, err = store.db.Exec(`UPDATE file_captures SET captures_json = ?`, []byte("{not json"))
	require.NoError(t, err)

	got, err := reg.ParseFile(ctx, pyPath, pySource)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	repaired, hit, err := store.GetFileCaptures(pyPath, sha)
	require.NoError(t, err)
	assert.True(t, hit)
	assert.Equal(t, want, *repaired)
}
