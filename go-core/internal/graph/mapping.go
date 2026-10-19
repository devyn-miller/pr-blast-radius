package graph

import (
	"cmp"
	"slices"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/gitdiff"
)

// MapHunksToSymbols is the general form of resolver.go's findEnclosingSymbol
// — given changed line ranges per file, find which already-indexed nodes
// in an already-built Graph overlap those ranges.
func MapHunksToSymbols(g *Graph, diffs []gitdiff.FileDiff) []ChangedSymbol {
	// Non-nil so an empty result marshals as [] rather than null; the
	// Python client decodes this as list[ChangedSymbol].
	changed := []ChangedSymbol{}
	seen := make(map[NodeID]bool)

	for _, diff := range diffs {
		for _, node := range g.Nodes {
			if node.FilePath != diff.FilePath {
				continue
			}
			for _, hunk := range diff.Hunks {
				if rangesOverlap(node.StartLine, node.EndLine, hunk.StartLine, hunk.EndLine) {
					if seen[node.ID] {
						continue
					}
					seen[node.ID] = true
					changed = append(changed, ChangedSymbol{
						SymbolID:   node.ID,
						ChangeType: classifyChange(diff.ChangeType, hunk, node),
					})
				}
			}
		}
	}

	// g.Nodes (and the diffs, which come from go-git's status map) iterate
	// in random order. Sort so RootSymbols, and therefore BFS order, the
	// path chosen per affected node and its severity, are reproducible.
	slices.SortFunc(changed, func(a, b ChangedSymbol) int {
		na, nb := g.Nodes[a.SymbolID], g.Nodes[b.SymbolID]
		return cmp.Or(
			cmp.Compare(na.FilePath, nb.FilePath),
			cmp.Compare(na.StartLine, nb.StartLine),
			cmp.Compare(a.SymbolID, b.SymbolID),
		)
	})
	return changed
}

// UnanalyzedDeletedFiles returns the paths of deleted files that have no nodes
// in g, sorted. The graph is built by walking the files currently on disk, so a
// file deleted in the diff is normally absent from it entirely: its symbols can
// be neither roots nor reachable, and that gap is invisible in the result
// unless it is reported explicitly. A deletion that was since restored on disk
// still has nodes and is not reported.
func UnanalyzedDeletedFiles(g *Graph, diffs []gitdiff.FileDiff) []string {
	withNodes := make(map[string]bool, len(g.Nodes))
	for _, node := range g.Nodes {
		withNodes[node.FilePath] = true
	}

	// Non-nil so an empty result marshals as [] rather than null.
	paths := []string{}
	for _, diff := range diffs {
		if diff.ChangeType == gitdiff.ChangeDeleted && !withNodes[diff.FilePath] {
			paths = append(paths, diff.FilePath)
		}
	}
	// go-git's status map iterates in random order.
	slices.Sort(paths)
	return paths
}

func rangesOverlap(aStart, aEnd, bStart, bEnd int) bool {
	return aStart <= bEnd && bStart <= aEnd
}

// classifyChange upgrades a plain Modified diff to SignatureChanged when
// the hunk overlaps the symbol's FIRST line — conservative by design: it
// can under-detect but never over-claims a body-only edit as
// signature-breaking.
func classifyChange(baseType gitdiff.ChangeType, hunk gitdiff.Hunk, node Node) ChangeType {
	switch baseType {
	case gitdiff.ChangeAdded:
		return ChangeAdded
	case gitdiff.ChangeDeleted:
		return ChangeDeleted
	}
	if hunk.StartLine <= node.StartLine {
		return ChangeSignatureChanged
	}
	return ChangeModified
}
