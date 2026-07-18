// go-core/internal/graph/traversal.go
package graph

import "time"

type ChangeType string

const (
	ChangeAdded            ChangeType = "Added"
	ChangeModified         ChangeType = "Modified"
	ChangeDeleted          ChangeType = "Deleted"
	ChangeSignatureChanged ChangeType = "SignatureChanged"
)

type ChangedSymbol struct {
	SymbolID   NodeID
	ChangeType ChangeType
}

type AffectedNode struct {
	NodeID          NodeID
	Distance        int
	Path            []NodeID
	SeverityScore   float64
	SeverityReasons []string
}

type BlastRadiusResult struct {
	RootSymbols         []ChangedSymbol
	AffectedNodes       []AffectedNode
	MaxDepthUsed        int
	TotalNodesAffected  int
	TotalEdgesTraversed int
	ExecutionTime       time.Duration
}

// SeverityScorer is the minimal interface graph needs from package
// severity — defined on the consumer side so graph never imports
// severity's concrete Weights type or constructor.
type SeverityScorer interface {
	Score(node Node, distance int, weakestConfidenceOnPath float32, maxInDegreeInGraph int) float64
	Reasons(node Node, distance int, weakestConfidenceOnPath float32) []string
}

// Traverser takes an ALREADY-BUILT *Graph as pure input — decoupled from
// BuildGraph so cycle-handling/BFS correctness are testable against
// small, hand-authored graph literals with zero dependency on parsing,
// git, or I/O of any kind.
type Traverser struct {
	g *Graph
}

func NewTraverser(g *Graph) *Traverser {
	return &Traverser{g: g}
}

type bfsQueueItem struct {
	nodeID        NodeID
	distance      int
	path          []NodeID
	weakestOnPath float32
}

// ComputeBlastRadius performs REVERSE BFS from every root (via g.InEdges —
// "who calls me"), bounded by maxDepth, with a visited-set keyed by
// NodeID. This is the only place cycle-safety is enforced in the whole
// system.
func (t *Traverser) ComputeBlastRadius(roots []ChangedSymbol, maxDepth int) BlastRadiusResult {
	start := time.Now()

	visited := make(map[NodeID]bool)
	var affected []AffectedNode
	edgesTraversed := 0

	queue := make([]bfsQueueItem, 0, len(roots))
	for _, root := range roots {
		visited[root.SymbolID] = true
		queue = append(queue, bfsQueueItem{
			nodeID: root.SymbolID, distance: 0,
			path: []NodeID{root.SymbolID}, weakestOnPath: 1.0,
		})
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.distance >= maxDepth {
			continue
		}

		for _, edge := range t.g.InEdges[item.nodeID] {
			edgesTraversed++
			caller := edge.From

			if visited[caller] {
				continue
			}
			visited[caller] = true

			weakest := item.weakestOnPath
			if edge.Confidence < weakest {
				weakest = edge.Confidence
			}

			newPath := append(append([]NodeID{}, item.path...), caller)

			affected = append(affected, AffectedNode{
				NodeID:   caller,
				Distance: item.distance + 1,
				Path:     newPath,
			})

			queue = append(queue, bfsQueueItem{
				nodeID: caller, distance: item.distance + 1,
				path: newPath, weakestOnPath: weakest,
			})
		}
	}

	return BlastRadiusResult{
		RootSymbols:         roots,
		AffectedNodes:       affected,
		MaxDepthUsed:        maxDepth,
		TotalNodesAffected:  len(affected),
		TotalEdgesTraversed: edgesTraversed,
		ExecutionTime:       time.Since(start),
	}
}

// PopulateSeverity fills in SeverityScore/SeverityReasons on an already-
// computed BlastRadiusResult. Kept as a SEPARATE pass so severity retuning
// never requires re-running traversal.
func (t *Traverser) PopulateSeverity(result *BlastRadiusResult, scorer SeverityScorer) {
	maxInDegree := 0
	for _, node := range t.g.Nodes {
		if node.InDegree > maxInDegree {
			maxInDegree = node.InDegree
		}
	}

	for i := range result.AffectedNodes {
		an := &result.AffectedNodes[i]
		node, ok := t.g.Nodes[an.NodeID]
		if !ok {
			continue
		}

		weakest := t.weakestConfidenceAlongPath(an.Path)
		an.SeverityScore = scorer.Score(node, an.Distance, weakest, maxInDegree)
		an.SeverityReasons = scorer.Reasons(node, an.Distance, weakest)
	}
}

// weakestConfidenceAlongPath walks a recorded path and finds the minimum
// edge confidence between consecutive nodes.
func (t *Traverser) weakestConfidenceAlongPath(path []NodeID) float32 {
	weakest := float32(1.0)
	for i := 0; i < len(path)-1; i++ {
		from, to := path[i+1], path[i]
		for _, e := range t.g.OutEdges[from] {
			if e.To == to && e.Confidence < weakest {
				weakest = e.Confidence
			}
		}
	}
	return weakest
}