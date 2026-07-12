package mcpserver

import (
	"os"
	"path/filepath"
	"testing"

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