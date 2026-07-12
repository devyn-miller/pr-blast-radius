package graph

import (
	"time"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
)

// NodeID uniquely identifies a symbol repo-wide. Constructed only via
// computeNodeID (id.go) — never assembled ad hoc, since every part of
// this system (cache keys, edge endpoints, traversal visited-sets)
// depends on identical inputs always producing an identical ID.
type NodeID string

type Node struct {
	ID            NodeID
	Kind          astparse.SymbolKind
	FilePath      string
	Name          string
	QualifiedName string
	Language      astparse.Language
	Exported      bool
	StartLine     int
	EndLine       int
	InDegree      int
	OutDegree     int
	Tags          []string
}

type EdgeType string

const (
	EdgeCalls      EdgeType = "Calls"
	EdgeImports    EdgeType = "Imports"
	EdgeImplements EdgeType = "Implements"
	EdgeRoutesTo   EdgeType = "RoutesTo"
	EdgeExtends    EdgeType = "Extends"
)

type CallSite struct {
	File string
	Line int
}

// Edge.Confidence is the single most important field this package
// produces (see resolver.go's rule table). It is NEVER left at a default
// zero value implicitly — every code path that constructs an Edge sets
// it explicitly, and resolver_test.go asserts the exact expected value
// for each resolution rule.
type Edge struct {
	From       NodeID
	To         NodeID
	Type       EdgeType
	Confidence float32
	Tags       []string
	Site       CallSite
}

type Metadata struct {
	RepoPath            string
	CommitSHA           string
	GeneratedAt         time.Time
	ToolVersion         string
	CacheHitRatio       float32
	UnresolvedCallCount int
}

// Graph is an adjacency-list representation with BOTH directions indexed.
// InEdges exists specifically because blast radius traversal is a REVERSE
// reachability problem ("who depends on me") — without a pre-built reverse
// index, that traversal would require an O(E) scan per hop, which matters
// once repo-scale graphs are in play.
type Graph struct {
	Nodes    map[NodeID]Node
	OutEdges map[NodeID][]Edge
	InEdges  map[NodeID][]Edge
	Metadata Metadata
}

func newEmptyGraph() *Graph {
	return &Graph{
		Nodes:    make(map[NodeID]Node),
		OutEdges: make(map[NodeID][]Edge),
		InEdges:  make(map[NodeID][]Edge),
	}
}

// addEdge is the ONLY place edges are inserted into a Graph, guaranteeing
// OutEdges and InEdges can never drift out of sync with each other — a
// class of bug that would otherwise be very easy to introduce silently
// (e.g. someone appending to OutEdges directly and forgetting InEdges).
func (g *Graph) addEdge(e Edge) {
	g.OutEdges[e.From] = append(g.OutEdges[e.From], e)
	g.InEdges[e.To] = append(g.InEdges[e.To], e)

	if n, ok := g.Nodes[e.From]; ok {
		n.OutDegree++
		g.Nodes[e.From] = n
	}
	if n, ok := g.Nodes[e.To]; ok {
		n.InDegree++
		g.Nodes[e.To] = n
	}
}