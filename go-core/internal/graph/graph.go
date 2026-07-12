package graph

import (
	"context"
	"fmt"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
)

// FileParser is the minimal interface BuildGraph needs from a parser
// registry. Both *astparse.Registry and *cache.CachedRegistry satisfy
// this — declared here, on the consumer side, so `graph` never imports
// `cache`, keeping the dependency direction one-way as designed.
type FileParser interface {
	ParseFile(ctx context.Context, filePath string, source []byte) (astparse.FileCaptures, error)
}

// FileSource is the minimal input BuildGraph needs per file. Deliberately
// NOT tied to gitdiff.FileDiff — BuildGraph constructs the FULL repo
// graph, independent of which files a particular PR changed.
type FileSource struct {
	Path   string
	Source []byte
}

// BuildGraph runs the full two-pass pipeline: parse + index every file
// (pass 1), finalize interface/implementer relationships, then resolve
// every file's calls into confidence-scored edges (pass 2).
func BuildGraph(ctx context.Context, registry FileParser, files []FileSource) (*Graph, error) {
	table := NewSymbolTable()
	var captures []astparse.FileCaptures

	for _, f := range files {
		fc, err := registry.ParseFile(ctx, f.Path, f.Source)
		if err != nil {
			if isUnsupportedFileErr(err) {
				continue // non-source files are expected, not an error condition
			}
			return nil, fmt.Errorf("graph: parsing %s: %w", f.Path, err)
		}
		fc.FilePath = f.Path // defensive: ensure downstream code always has this set
		table.Index(fc)
		captures = append(captures, fc)
	}

	interfaceMethodSets := make(map[string][]string)
	for _, fc := range captures {
		for _, iface := range fc.Interfaces {
			interfaceMethodSets[iface.Name] = iface.Methods
		}
	}
	table.Finalize(interfaceMethodSets)

	g := newEmptyGraph()

	// Register every Node BEFORE resolving any edges — enforced
	// structurally, not by convention, since an edge referencing a
	// not-yet-registered node is a real bug class.
	for _, fc := range captures {
		for _, sym := range fc.Symbols {
			qualifiedName := buildQualifiedName(sym.Name, sym.QualifiedHint)
			id := computeNodeID(fc.FilePath, qualifiedName)
			g.Nodes[id] = Node{
				ID: id, Kind: sym.Kind, FilePath: fc.FilePath, Name: sym.Name,
				QualifiedName: qualifiedName, Language: fc.Language,
				Exported: sym.Exported, StartLine: sym.StartLine, EndLine: sym.EndLine,
				Tags: sym.DecoratorTags,
			}
		}
	}

	resolver := NewResolver(table)
	totalUnresolved := 0

	for _, fc := range captures {
		edges, unresolved := resolver.ResolveEdges(fc)
		totalUnresolved += unresolved
		for _, e := range edges {
			if e.To == unresolvedSentinelID {
				ensureSentinelNode(g)
			}
			g.addEdge(e)
		}
	}

	g.Metadata.UnresolvedCallCount = totalUnresolved
	return g, nil
}

func ensureSentinelNode(g *Graph) {
	if _, exists := g.Nodes[unresolvedSentinelID]; exists {
		return
	}
	g.Nodes[unresolvedSentinelID] = Node{
		ID: unresolvedSentinelID, Kind: "Unknown", Name: "<unresolved reflective target>",
	}
}

func isUnsupportedFileErr(err error) bool {
	_, ok := err.(*astparse.UnsupportedFileError)
	return ok
}