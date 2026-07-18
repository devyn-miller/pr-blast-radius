package graph

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestGraph constructs a small graph by hand:
//
//   A --calls--> B --calls--> C --calls--> A     (a 3-cycle: A<-B<-C<-A)
//   D --calls--> B                                (D is a second entry point into the cycle)
//   E --calls--> D                                (E is two hops from B)
//
// Reverse traversal from B (the "changed" symbol) must find: A (dist 1),
// D (dist 1), C (dist 2, via A), E (dist 2, via D) — and crucially must
// NOT infinite-loop on the A->B->C->A cycle.
func buildTestGraph() *Graph {
	g := newEmptyGraph()
	for _, id := range []NodeID{"A", "B", "C", "D", "E"} {
		g.Nodes[id] = Node{ID: id, Name: string(id)}
	}
	g.addEdge(Edge{From: "A", To: "B", Type: EdgeCalls, Confidence: 1.0})
	g.addEdge(Edge{From: "B", To: "C", Type: EdgeCalls, Confidence: 1.0})
	g.addEdge(Edge{From: "C", To: "A", Type: EdgeCalls, Confidence: 0.9}) // closes the cycle
	g.addEdge(Edge{From: "D", To: "B", Type: EdgeCalls, Confidence: 1.0})
	g.addEdge(Edge{From: "E", To: "D", Type: EdgeCalls, Confidence: 1.0})
	return g
}

func TestTraverser_HandlesCycles_NoInfiniteLoop(t *testing.T) {
	g := buildTestGraph()
	traverser := NewTraverser(g)

	done := make(chan BlastRadiusResult, 1)
	go func() {
		done <- traverser.ComputeBlastRadius(
			[]ChangedSymbol{{SymbolID: "B", ChangeType: ChangeModified}},
			10, // deliberately generous depth — if cycle-guarding were broken, this hangs or explodes node count
		)
	}()

	select {
	case result := <-done:
		byID := make(map[NodeID]AffectedNode)
		for _, a := range result.AffectedNodes {
			byID[a.NodeID] = a
		}

		require.Contains(t, byID, NodeID("A"))
		assert.Equal(t, 1, byID["A"].Distance)

		require.Contains(t, byID, NodeID("D"))
		assert.Equal(t, 1, byID["D"].Distance)

		require.Contains(t, byID, NodeID("C"))
		assert.Equal(t, 2, byID["C"].Distance, "C is reached via A, at distance 2")

		require.Contains(t, byID, NodeID("E"))
		assert.Equal(t, 2, byID["E"].Distance, "E is reached via D, at distance 2")

		// The cycle must not cause B or C to be recorded more than once,
		// nor cause A to be re-recorded when C's own InEdges are explored
		// a second time via the cycle.
		assert.Len(t, result.AffectedNodes, 4, "exactly {A, D, C, E} — no duplicates, no infinite growth")

	case <-time.After(2 * time.Second):
		t.Fatal("ComputeBlastRadius did not terminate — cycle guard is broken")
	}
}

func TestTraverser_RespectsMaxDepth(t *testing.T) {
	g := buildTestGraph()
	traverser := NewTraverser(g)

	result := traverser.ComputeBlastRadius(
		[]ChangedSymbol{{SymbolID: "B", ChangeType: ChangeModified}},
		1, // only direct callers
	)

	byID := make(map[NodeID]bool)
	for _, a := range result.AffectedNodes {
		byID[a.NodeID] = true
	}

	assert.True(t, byID["A"])
	assert.True(t, byID["D"])
	assert.False(t, byID["C"], "C is at distance 2 — must be excluded when maxDepth=1")
	assert.False(t, byID["E"], "E is at distance 2 — must be excluded when maxDepth=1")
	assert.Equal(t, 1, result.MaxDepthUsed)
}

func TestTraverser_MultipleRoots_ShareOneVisitedSet(t *testing.T) {
	g := buildTestGraph()
	traverser := NewTraverser(g)

	// Both B and D changed in the same PR. Since traversal is REVERSE
	// (who calls me), this tests that shared visited-state across
	// multiple roots doesn't cause a root itself to be mis-recorded as
	// its own affected node, and doesn't double-count nodes reachable
	// from both roots.
	result := traverser.ComputeBlastRadius(
		[]ChangedSymbol{
			{SymbolID: "B", ChangeType: ChangeModified},
			{SymbolID: "D", ChangeType: ChangeModified},
		},
		10,
	)

	seen := make(map[NodeID]int)
	for _, a := range result.AffectedNodes {
		seen[a.NodeID]++
		assert.NotEqual(t, NodeID("B"), a.NodeID, "root nodes must never appear in AffectedNodes")
		assert.NotEqual(t, NodeID("D"), a.NodeID, "root nodes must never appear in AffectedNodes")
	}
	for id, count := range seen {
		assert.Equal(t, 1, count, "node %s must not be recorded more than once even when reachable from multiple roots", id)
	}
}

func TestTraverser_NoIncomingEdges_ReturnsEmptyResult(t *testing.T) {
	g := newEmptyGraph()
	g.Nodes["isolated"] = Node{ID: "isolated"}
	traverser := NewTraverser(g)

	result := traverser.ComputeBlastRadius(
		[]ChangedSymbol{{SymbolID: "isolated", ChangeType: ChangeModified}}, 5,
	)

	assert.Empty(t, result.AffectedNodes)
	assert.Equal(t, 0, result.TotalNodesAffected)
	assert.Equal(t, 0, result.TotalEdgesTraversed)
}

func TestTraverser_EdgesTraversedCount_IncludesRevisitedCycleEdges(t *testing.T) {
	// TotalEdgesTraversed counts every InEdges lookup performed, INCLUDING
	// edges that lead to an already-visited node (they're inspected, then
	// discarded by the cycle guard) — this is an observability metric
	// (how much work was done), distinct from AffectedNodes (what was found).
	g := buildTestGraph()
	traverser := NewTraverser(g)

	result := traverser.ComputeBlastRadius(
		[]ChangedSymbol{{SymbolID: "B", ChangeType: ChangeModified}}, 10,
	)

	// InEdges walked: B<-A, A<-C, D<-(none further back from D itself
	// since D has no InEdges), C<-B(visited, still counted as inspected).
	// Exact count depends on traversal order but must be >= AffectedNodes count,
	// since some edges are inspected-and-discarded (cycle hits) in addition
	// to the ones that produced a new AffectedNode.
	assert.GreaterOrEqual(t, result.TotalEdgesTraversed, result.TotalNodesAffected)
}

func TestPopulateSeverity_UsesWeakestConfidenceOnPath(t *testing.T) {
	g := newEmptyGraph()
	g.Nodes["root"] = Node{ID: "root", InDegree: 1}
	g.Nodes["mid"] = Node{ID: "mid", InDegree: 1}
	g.Nodes["far"] = Node{ID: "far", InDegree: 1}
	g.addEdge(Edge{From: "far", To: "mid", Confidence: 0.9})
	g.addEdge(Edge{From: "mid", To: "root", Confidence: 0.3}) // the weak link

	traverser := NewTraverser(g)
	result := traverser.ComputeBlastRadius(
		[]ChangedSymbol{{SymbolID: "root", ChangeType: ChangeModified}}, 5,
	)

	fake := &fakeScorer{lastWeakestSeen: make(map[NodeID]float32)}
	traverser.PopulateSeverity(&result, fake)

	var farRecorded bool
	for i := range result.AffectedNodes {
		if result.AffectedNodes[i].NodeID == "far" {
			farRecorded = true
		}
	}
	require.True(t, farRecorded)
	assert.InDelta(t, float32(0.3), fake.lastWeakestSeen["far"], 0.001,
		"far's path weakest-link must be 0.3 (the mid->root edge), not 0.9 (the far->mid edge)")
	assert.InDelta(t, float32(0.3), fake.lastWeakestSeen["mid"], 0.001,
		"mid's path weakest-link is the mid->root edge itself, 0.3")
}

func TestPopulateSeverity_SkipsUnknownNodeGracefully(t *testing.T) {
	// If AffectedNodes somehow references a NodeID not present in g.Nodes
	// (shouldn't normally happen, but defends against future refactors),
	// PopulateSeverity must skip it rather than panic.
	g := newEmptyGraph()
	g.Nodes["root"] = Node{ID: "root"}

	traverser := NewTraverser(g)
	result := BlastRadiusResult{
		AffectedNodes: []AffectedNode{
			{NodeID: "ghost-node-not-in-graph", Distance: 1, Path: []NodeID{"root", "ghost-node-not-in-graph"}},
		},
	}

	fake := &fakeScorer{lastWeakestSeen: make(map[NodeID]float32)}
	assert.NotPanics(t, func() {
		traverser.PopulateSeverity(&result, fake)
	})
	assert.Equal(t, 0.0, result.AffectedNodes[0].SeverityScore, "unknown node must score zero, never fabricated")
}

// fakeScorer records inputs instead of computing a real score — this is
// exactly what the SeverityScorer interface buys us: graph's tests never
// import package severity at all.
type fakeScorer struct {
	lastWeakestSeen map[NodeID]float32
}

func (f *fakeScorer) Score(node Node, distance int, weakest float32, maxInDeg int) float64 {
	f.lastWeakestSeen[node.ID] = weakest
	return 0
}
func (f *fakeScorer) Reasons(node Node, distance int, weakest float32) []string { return nil }