package severity

import (
	"testing"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/graph"
	"github.com/stretchr/testify/assert"
)

func TestDefaultWeights_SumToOne(t *testing.T) {
	sum := DefaultWeights.Distance + DefaultWeights.FanIn +
		DefaultWeights.RouteHandler + DefaultWeights.Confidence
	assert.InDelta(t, 1.0, sum, 0.0001)
}

func TestScore_IsBoundedZeroToOne(t *testing.T) {
	scorer := NewScorer(DefaultWeights)

	// Deliberately adversarial inputs: max possible fan-in, zero distance,
	// route handler, perfect confidence — should land exactly at 1.0,
	// not overflow it.
	node := graph.Node{InDegree: 100, Tags: []string{"route:POST:/orders"}}
	score := scorer.Score(node, 0, 1.0, 100)
	assert.LessOrEqual(t, score, 1.0)
	assert.GreaterOrEqual(t, score, 0.0)

	// Worst case: far away, isolated, no route, no confidence.
	farNode := graph.Node{InDegree: 0}
	farScore := scorer.Score(farNode, 10, 0.0, 100)
	assert.GreaterOrEqual(t, farScore, 0.0)
	assert.Less(t, farScore, score, "a distant, isolated, non-route node must score strictly lower than the adversarial max case")
}

func TestScore_CloserNodesScoreHigherAllElseEqual(t *testing.T) {
	scorer := NewScorer(DefaultWeights)
	node := graph.Node{InDegree: 3}

	near := scorer.Score(node, 1, 0.9, 10)
	far := scorer.Score(node, 5, 0.9, 10)

	assert.Greater(t, near, far, "distance term must strictly decrease severity as distance grows")
}

func TestScore_RouteHandlerScoresHigherThanNonRouteAllElseEqual(t *testing.T) {
	scorer := NewScorer(DefaultWeights)

	routeNode := graph.Node{InDegree: 2, Tags: []string{"route:GET:/inventory/:sku"}}
	plainNode := graph.Node{InDegree: 2}

	routeScore := scorer.Score(routeNode, 2, 0.9, 10)
	plainScore := scorer.Score(plainNode, 2, 0.9, 10)

	assert.Greater(t, routeScore, plainScore)
}

func TestReasons_TagsRouteHandlerViaDecoratorTag(t *testing.T) {
	scorer := NewScorer(DefaultWeights)
	node := graph.Node{Tags: []string{"route:POST:/orders"}}

	reasons := scorer.Reasons(node, 1, 0.9)
	assert.Contains(t, reasons, "is_route_handler")
}

func TestReasons_TagsHighFanIn(t *testing.T) {
	scorer := NewScorer(DefaultWeights)

	assert.Contains(t, scorer.Reasons(graph.Node{InDegree: 6}, 1, 1.0), "high_fan_in")
	assert.NotContains(t, scorer.Reasons(graph.Node{InDegree: 4}, 1, 1.0), "high_fan_in")
}

func TestReasons_TagsLowConfidencePath(t *testing.T) {
	scorer := NewScorer(DefaultWeights)

	assert.Contains(t, scorer.Reasons(graph.Node{}, 1, 0.3), "low_confidence_path")
	assert.NotContains(t, scorer.Reasons(graph.Node{}, 1, 0.9), "low_confidence_path")
}

func TestReasons_TagsPublicAPIWhenExported(t *testing.T) {
	scorer := NewScorer(DefaultWeights)

	assert.Contains(t, scorer.Reasons(graph.Node{Exported: true}, 1, 1.0), "public_api")
	assert.NotContains(t, scorer.Reasons(graph.Node{Exported: false}, 1, 1.0), "public_api")
}

// Table-driven test across the full weight space — this is the kind of
// test that's cheap to write here BECAUSE Score/Reasons take everything
// as plain parameters, with no fixture graph or traversal required.
func TestScore_TableDriven(t *testing.T) {
	scorer := NewScorer(DefaultWeights)

	cases := []struct {
		name       string
		node       graph.Node
		distance   int
		confidence float32
		maxInDeg   int
		wantMin    float64
		wantMax    float64
	}{
		{"root-adjacent, isolated, no route, uncertain path", graph.Node{}, 1, 0.3, 10, 0.0, 0.3},
		{"far, hub, route handler, certain path", graph.Node{InDegree: 10, Tags: []string{"route:GET:/x"}}, 5, 1.0, 10, 0.4, 0.71}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scorer.Score(tc.node, tc.distance, tc.confidence, tc.maxInDeg)
			assert.GreaterOrEqual(t, got, tc.wantMin)
			assert.LessOrEqual(t, got, tc.wantMax)
		})
	}
}
func TestReasons_NoneApply_IsEmptyNotNil(t *testing.T) {
	// Marshals into AffectedNode.SeverityReasons; nil would encode as null,
	// which the Python client rejects.
	reasons := NewScorer(DefaultWeights).Reasons(graph.Node{}, 1, 1.0)
	assert.NotNil(t, reasons)
	assert.Empty(t, reasons)
}

// TestMaxReachableScore_MatchesScorerAtMaximalInputs pins the closed-form
// ceiling in MaxReachableScore to what Score actually returns when every term
// is maximal, so the two cannot drift. It also prints the ceilings, which are
// what the Python risk-tier cutoffs are calibrated against.
func TestMaxReachableScore_MatchesScorerAtMaximalInputs(t *testing.T) {
	scorer := NewScorer(DefaultWeights)

	const maxInDegree = 10
	routeNode := graph.Node{InDegree: maxInDegree, Tags: []string{"route:GET:/x"}}
	plainNode := graph.Node{InDegree: maxInDegree}

	// distance 1 is the smallest distance an affected node can have; distance 0
	// is a changed root, which PopulateSeverity never scores.
	gotRoute := scorer.Score(routeNode, 1, 1.0, maxInDegree)
	gotPlain := scorer.Score(plainNode, 1, 1.0, maxInDegree)

	t.Logf("max reachable severity score at distance>=1: route=%.4f non-route=%.4f",
		gotRoute, gotPlain)

	assert.InDelta(t, MaxReachableScore(DefaultWeights, true), gotRoute, 1e-9)
	assert.InDelta(t, MaxReachableScore(DefaultWeights, false), gotPlain, 1e-9)

	// A larger distance can only lower the score, never raise it past the ceiling.
	for d := 2; d <= 6; d++ {
		assert.LessOrEqual(t, scorer.Score(routeNode, d, 1.0, maxInDegree),
			MaxReachableScore(DefaultWeights, true))
	}
}
