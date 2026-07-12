package severity

import (
	"strings"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/graph"
)

// Weights are the four coefficients from the severity formula documented
// in the requirements analysis:
//
//   severity_score = w1 * (1 / (1 + distance))
//                  + w2 * normalized(in_degree)
//                  + w3 * (1.0 if is_route_handler else 0.0)
//                  + w4 * confidence_of_weakest_edge_on_path
//
// Exposed as a struct (not four package-level consts) specifically so a
// future experiment ("what if we weight fan-in higher for a demo repo
// with one obvious hotspot") is a one-line change at the call site, not
// a recompile-everywhere constant edit.
type Weights struct {
	Distance     float64 // w1
	FanIn        float64 // w2
	RouteHandler float64 // w3
	Confidence   float64 // w4
}

// DefaultWeights sums to 1.0 by construction — not enforced by the type
// system, but asserted in scorer_test.go, since a severity_score outside
// [0,1] would break the Mermaid IR's risk_tier bucketing downstream.
var DefaultWeights = Weights{
	Distance:     0.35,
	FanIn:        0.35,
	RouteHandler: 0.20,
	Confidence:   0.10,
}

// Scorer is a stateless value once constructed — Score/Reasons take every
// input they need as parameters, making them trivially table-testable
// without any setup/fixture graph required.
type Scorer struct {
	weights Weights
}

func NewScorer(w Weights) *Scorer {
	return &Scorer{weights: w}
}

// Score computes severity_score in [0, 1]. maxInDegreeInGraph is passed in
// (rather than looked up internally) so this function stays a pure
// function of its arguments — the caller (PopulateSeverity, below) computes
// it once per BlastRadiusResult rather than re-scanning the whole graph
// per node.
func (s *Scorer) Score(node graph.Node, distance int, weakestConfidenceOnPath float32, maxInDegreeInGraph int) float64 {
	distanceTerm := 1.0 / (1.0 + float64(distance))

	fanInTerm := 0.0
	if maxInDegreeInGraph > 0 {
		fanInTerm = float64(node.InDegree) / float64(maxInDegreeInGraph)
	}

	routeTerm := 0.0
	if isRouteHandler(node) {
		routeTerm = 1.0
	}

	confidenceTerm := float64(weakestConfidenceOnPath)

	score := s.weights.Distance*distanceTerm +
		s.weights.FanIn*fanInTerm +
		s.weights.RouteHandler*routeTerm +
		s.weights.Confidence*confidenceTerm

	return clamp01(score)
}

// Reasons returns the human-readable tags that justify a score — these
// map directly onto BlastRadiusResult.AffectedNode.SeverityReasons in the
// JSON schema, and onto NarrativeItem.risk_reasons in the LLM's Pydantic
// IR one layer up. Kept as a SEPARATE function from Score deliberately:
// the numeric score and its explanation are different concerns, and a
// reviewer should be able to unit test "does this produce the right tag"
// without asserting on a specific float value.
func (s *Scorer) Reasons(node graph.Node, distance int, weakestConfidenceOnPath float32) []string {
	var reasons []string

	if isRouteHandler(node) {
		reasons = append(reasons, "is_route_handler")
	}
	if node.InDegree >= highFanInThreshold {
		reasons = append(reasons, "high_fan_in")
	}
	if weakestConfidenceOnPath < lowConfidenceThreshold {
		reasons = append(reasons, "low_confidence_path")
	}
	if strings.Contains(strings.ToLower(node.FilePath), "test") {
		reasons = append(reasons, "touches_test")
	}
	if node.Exported {
		reasons = append(reasons, "public_api")
	}

	return reasons
}

const (
	highFanInThreshold     = 5
	lowConfidenceThreshold = 0.5
)

func isRouteHandler(node graph.Node) bool {
	for _, tag := range node.Tags {
		if strings.HasPrefix(tag, "route:") {
			return true
		}
	}
	return node.Kind == "RouteHandler"
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}