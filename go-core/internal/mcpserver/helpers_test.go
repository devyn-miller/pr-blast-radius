package mcpserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/gitdiff"
	"github.com/devyn-miller/pr-blast-radius/go-core/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSymbolSource_TruncatesAtMaxBytes(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\nline3\nline4\nline5\n"
	path := filepath.Join(dir, "f.go")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	node := graph.Node{FilePath: "f.go", StartLine: 1, EndLine: 5}

	src, truncated, err := readSymbolSource(dir, node, 10)
	require.NoError(t, err)
	assert.True(t, truncated)
	assert.Len(t, src, 10)

	src2, truncated2, err := readSymbolSource(dir, node, 10000)
	require.NoError(t, err)
	assert.False(t, truncated2)
	assert.Contains(t, src2, "line5")
}
func TestParseRefRange(t *testing.T) {
	cases := []struct {
		in   string
		want gitdiff.RefRange
	}{
		{"WORKING_TREE", gitdiff.RefRange{Kind: gitdiff.RefRangeWorkingTree}},
		{"HEAD~1", gitdiff.RefRange{Kind: gitdiff.RefRangeSingleCommit, ToRef: "HEAD~1"}},
		{"HEAD~1..HEAD", gitdiff.RefRange{Kind: gitdiff.RefRangeCommitRange, FromRef: "HEAD~1", ToRef: "HEAD"}},
		{"main...feature", gitdiff.RefRange{Kind: gitdiff.RefRangeCommitRange, FromRef: "main", ToRef: "feature"}},
		{"main..", gitdiff.RefRange{Kind: gitdiff.RefRangeCommitRange, FromRef: "main", ToRef: "HEAD"}},
		{"..feature", gitdiff.RefRange{Kind: gitdiff.RefRangeCommitRange, FromRef: "HEAD", ToRef: "feature"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, parseRefRange(tc.in))
		})
	}
}
