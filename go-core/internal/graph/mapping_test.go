package graph

import (
	"fmt"
	"testing"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/gitdiff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapHunksToSymbols_ClassifiesSignatureVsBodyChange(t *testing.T) {
	g := newEmptyGraph()
	g.Nodes["fn"] = Node{ID: "fn", FilePath: "engine.py", StartLine: 10, EndLine: 20}

	diffs := []gitdiff.FileDiff{
		{
			FilePath:   "engine.py",
			ChangeType: gitdiff.ChangeModified,
			Hunks:      []gitdiff.Hunk{{StartLine: 10, EndLine: 11}}, // touches the def line itself
		},
	}
	changed := MapHunksToSymbols(g, diffs)

	assert.Len(t, changed, 1)
	assert.Equal(t, ChangeSignatureChanged, changed[0].ChangeType)
}

func TestMapHunksToSymbols_BodyOnlyEditStaysModified(t *testing.T) {
	g := newEmptyGraph()
	g.Nodes["fn"] = Node{ID: "fn", FilePath: "engine.py", StartLine: 10, EndLine: 20}

	diffs := []gitdiff.FileDiff{
		{
			FilePath:   "engine.py",
			ChangeType: gitdiff.ChangeModified,
			Hunks:      []gitdiff.Hunk{{StartLine: 15, EndLine: 16}}, // mid-body only
		},
	}
	changed := MapHunksToSymbols(g, diffs)

	assert.Len(t, changed, 1)
	assert.Equal(t, ChangeModified, changed[0].ChangeType)
}

func TestMapHunksToSymbols_AddedFile_AllOverlappingSymbolsMarkedAdded(t *testing.T) {
	g := newEmptyGraph()
	g.Nodes["fn"] = Node{ID: "fn", FilePath: "new_file.go", StartLine: 1, EndLine: 5}

	diffs := []gitdiff.FileDiff{
		{
			FilePath:   "new_file.go",
			ChangeType: gitdiff.ChangeAdded,
			Hunks:      []gitdiff.Hunk{{StartLine: 1, EndLine: 5}},
		},
	}
	changed := MapHunksToSymbols(g, diffs)

	assert.Len(t, changed, 1)
	assert.Equal(t, ChangeAdded, changed[0].ChangeType,
		"Added files should classify as Added regardless of which line the hunk touches")
}

func TestMapHunksToSymbols_DeletedFile_AllOverlappingSymbolsMarkedDeleted(t *testing.T) {
	g := newEmptyGraph()
	g.Nodes["fn"] = Node{ID: "fn", FilePath: "old_file.go", StartLine: 1, EndLine: 5}

	diffs := []gitdiff.FileDiff{
		{
			FilePath:   "old_file.go",
			ChangeType: gitdiff.ChangeDeleted,
			Hunks:      []gitdiff.Hunk{{StartLine: 1, EndLine: 5}},
		},
	}
	changed := MapHunksToSymbols(g, diffs)

	assert.Len(t, changed, 1)
	assert.Equal(t, ChangeDeleted, changed[0].ChangeType)
}

func TestMapHunksToSymbols_NoOverlap_ProducesNoChangedSymbols(t *testing.T) {
	g := newEmptyGraph()
	g.Nodes["fn"] = Node{ID: "fn", FilePath: "engine.py", StartLine: 10, EndLine: 20}

	diffs := []gitdiff.FileDiff{
		{
			FilePath:   "engine.py",
			ChangeType: gitdiff.ChangeModified,
			Hunks:      []gitdiff.Hunk{{StartLine: 50, EndLine: 55}}, // nowhere near the symbol
		},
	}
	changed := MapHunksToSymbols(g, diffs)

	assert.Empty(t, changed)
}

func TestMapHunksToSymbols_DifferentFile_IsIgnored(t *testing.T) {
	g := newEmptyGraph()
	g.Nodes["fn"] = Node{ID: "fn", FilePath: "engine.py", StartLine: 10, EndLine: 20}

	diffs := []gitdiff.FileDiff{
		{
			FilePath:   "unrelated.py", // different file entirely
			ChangeType: gitdiff.ChangeModified,
			Hunks:      []gitdiff.Hunk{{StartLine: 10, EndLine: 11}},
		},
	}
	changed := MapHunksToSymbols(g, diffs)

	assert.Empty(t, changed)
}

func TestMapHunksToSymbols_MultipleOverlappingSymbols_BothIncluded(t *testing.T) {
	// A hunk spanning a range that overlaps TWO distinct symbols (e.g. an
	// edit that touches the tail of one function and the head of the
	// next) must flag both, not just the first match found.
	g := newEmptyGraph()
	g.Nodes["fnA"] = Node{ID: "fnA", FilePath: "engine.py", StartLine: 1, EndLine: 10}
	g.Nodes["fnB"] = Node{ID: "fnB", FilePath: "engine.py", StartLine: 11, EndLine: 20}

	diffs := []gitdiff.FileDiff{
		{
			FilePath:   "engine.py",
			ChangeType: gitdiff.ChangeModified,
			Hunks:      []gitdiff.Hunk{{StartLine: 9, EndLine: 12}}, // straddles both
		},
	}
	changed := MapHunksToSymbols(g, diffs)

	assert.Len(t, changed, 2)
	ids := []NodeID{changed[0].SymbolID, changed[1].SymbolID}
	assert.ElementsMatch(t, []NodeID{"fnA", "fnB"}, ids)
}

func TestMapHunksToSymbols_DeduplicatesSameSymbolAcrossMultipleHunks(t *testing.T) {
	// If a file has TWO separate hunks that both overlap the SAME symbol
	// (e.g. two non-contiguous edits inside one function), that symbol
	// must appear exactly once in the result, not twice.
	g := newEmptyGraph()
	g.Nodes["fn"] = Node{ID: "fn", FilePath: "engine.py", StartLine: 1, EndLine: 20}

	diffs := []gitdiff.FileDiff{
		{
			FilePath:   "engine.py",
			ChangeType: gitdiff.ChangeModified,
			Hunks: []gitdiff.Hunk{
				{StartLine: 3, EndLine: 4},
				{StartLine: 15, EndLine: 16},
			},
		},
	}
	changed := MapHunksToSymbols(g, diffs)

	assert.Len(t, changed, 1, "same symbol touched by two hunks must be deduplicated")
}
func TestMapHunksToSymbols_OrderIsDeterministic_ByFileThenLine(t *testing.T) {
	// g.Nodes is a map, so without an explicit sort the result order (and
	// with it BFS order and the path chosen per affected node) would vary
	// run to run. IDs are chosen so ID order differs from source order.
	g := newEmptyGraph()
	want := []NodeID{}
	for i, f := range []string{"a.py", "b.py"} {
		for line := 1; line <= 40; line += 4 {
			id := NodeID(fmt.Sprintf("z%02d_%s", 99-line-i, f))
			g.Nodes[id] = Node{ID: id, FilePath: f, StartLine: line, EndLine: line + 2}
			want = append(want, id)
		}
	}
	diffs := []gitdiff.FileDiff{
		{FilePath: "b.py", ChangeType: gitdiff.ChangeModified, Hunks: []gitdiff.Hunk{{StartLine: 1, EndLine: 40}}},
		{FilePath: "a.py", ChangeType: gitdiff.ChangeModified, Hunks: []gitdiff.Hunk{{StartLine: 1, EndLine: 40}}},
	}

	for run := 0; run < 50; run++ {
		changed := MapHunksToSymbols(g, diffs)
		got := make([]NodeID, len(changed))
		for i, c := range changed {
			got[i] = c.SymbolID
		}
		require.Equal(t, want, got, "run %d", run)
	}
}
