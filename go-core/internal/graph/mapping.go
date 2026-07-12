package graph

import "github.com/devyn-miller/pr-blast-radius/go-core/internal/gitdiff"

// MapHunksToSymbols is the general form of resolver.go's findEnclosingSymbol
// — given changed line ranges per file, find which already-indexed nodes
// in an already-built Graph overlap those ranges.
func MapHunksToSymbols(g *Graph, diffs []gitdiff.FileDiff) []ChangedSymbol {
	var changed []ChangedSymbol
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
	return changed
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