package gitdiff

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initRepoWithFiles creates a real git repo in a temp dir with one commit
// containing files, keyed by repo-relative path.
func initRepoWithFiles(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	for path, content := range files {
		full := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	require.NoError(t, wt.AddWithOptions(&git.AddOptions{All: true}))
	sig := &object.Signature{Name: "test", Email: "test@localhost", When: time.Unix(0, 0)}
	_, err = wt.Commit("base", &git.CommitOptions{Author: sig})
	require.NoError(t, err)

	return dir
}

func findDiff(diffs []FileDiff, path string) (FileDiff, bool) {
	for _, d := range diffs {
		if d.FilePath == path {
			return d, true
		}
	}
	return FileDiff{}, false
}

// TestExtractDiff_WorkingTree_DeletedFileIsReportedNotAnError covers the case
// that used to fail with "gitdiff: reading worktree file ...: no such file":
// the file is gone from disk, so there is nothing to read as its new content.
func TestExtractDiff_WorkingTree_DeletedFileIsReportedNotAnError(t *testing.T) {
	const deleted = "services/order_service.py"
	dir := initRepoWithFiles(t, map[string]string{
		deleted:            "def create_order():\n    return 1\n",
		"services/keep.py": "def keep():\n    return 2\n",
	})

	require.NoError(t, os.Remove(filepath.Join(dir, deleted)))

	diffs, headSHA, err := NewGoGitExtractor().ExtractDiff(
		context.Background(), dir, RefRange{Kind: RefRangeWorkingTree})

	require.NoError(t, err)
	assert.NotEmpty(t, headSHA)

	d, ok := findDiff(diffs, deleted)
	require.True(t, ok, "deleted file %s must be reported in the diff, got %+v", deleted, diffs)
	assert.Equal(t, ChangeDeleted, d.ChangeType)

	// New content is empty, so the whole old file (2 lines) is one hunk.
	require.Len(t, d.Hunks, 1)
	assert.Equal(t, Hunk{StartLine: 1, EndLine: 2}, d.Hunks[0])
}

// A deletion that has already been staged (`git rm` / `git add` of a deletion)
// reports Staging=Deleted with Worktree=Unmodified, which is the form that
// reached the worktree read and crashed.
func TestExtractDiff_WorkingTree_StagedDeletionIsClassifiedDeleted(t *testing.T) {
	const deleted = "shared/money.py"
	dir := initRepoWithFiles(t, map[string]string{
		deleted: "class Money:\n    pass\n",
	})

	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Remove(deleted)
	require.NoError(t, err)

	diffs, _, err := NewGoGitExtractor().ExtractDiff(
		context.Background(), dir, RefRange{Kind: RefRangeWorkingTree})

	require.NoError(t, err)
	d, ok := findDiff(diffs, deleted)
	require.True(t, ok, "staged deletion of %s must be reported, got %+v", deleted, diffs)
	assert.Equal(t, ChangeDeleted, d.ChangeType)
	assert.NotEmpty(t, d.Hunks)
}

func TestDiffTextToHunks_EmptyNewContentCoversWholeOldFile(t *testing.T) {
	hunks := diffTextToHunks("a\nb\nc\n", "")

	require.Len(t, hunks, 1)
	assert.Equal(t, Hunk{StartLine: 1, EndLine: 3}, hunks[0])
}
