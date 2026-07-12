package graph

import (
	"testing"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSymbolTable_ImplementersOf_FindsAllMatchingStructs(t *testing.T) {
	table := NewSymbolTable()

	// Two structs, both implementing a "Notifier" interface's method set,
	// one that does NOT (missing a required method).
	table.Index(astparse.FileCaptures{
		FilePath: "notifiers.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "Notify", Kind: astparse.KindMethod, QualifiedHint: "EmailNotifier"},
			{Name: "Notify", Kind: astparse.KindMethod, QualifiedHint: "SMSNotifier"},
			{Name: "Log", Kind: astparse.KindMethod, QualifiedHint: "Logger"}, // no Notify -> not an implementer
		},
	})

	table.Finalize(map[string][]string{
		"Notifier": {"Notify"},
	})

	implementers := table.ImplementersOf("Notifier")

	assert.ElementsMatch(t, []string{"EmailNotifier", "SMSNotifier"}, implementers)
	assert.NotContains(t, implementers, "Logger")
}

func TestResolver_R1_DirectCall_UnambiguousResolution(t *testing.T) {
	table := NewSymbolTable()
	table.Index(astparse.FileCaptures{
		FilePath: "utils.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "RoundCurrency", Kind: astparse.KindFunction},
		},
	})
	table.Finalize(nil)

	fc := astparse.FileCaptures{
		FilePath: "order_service.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "Create", Kind: astparse.KindMethod, QualifiedHint: "OrderService", StartLine: 1, EndLine: 10},
		},
		Calls: []astparse.CallCapture{
			{Kind: astparse.CallDirect, DirectName: "RoundCurrency", SiteLine: 5},
		},
	}

	resolver := NewResolver(table)
	edges, unresolved := resolver.ResolveEdges(fc)

	require.Len(t, edges, 1)
	assert.Equal(t, ConfidenceStaticDirect, edges[0].Confidence)
	assert.Equal(t, computeNodeID("utils.go", "RoundCurrency"), edges[0].To)
	assert.Equal(t, computeNodeID("order_service.go", "OrderService.Create"), edges[0].From,
		"edge must originate from the SPECIFIC enclosing method containing the call, not a file-level fallback")
	assert.Equal(t, 0, unresolved)
}

func TestResolver_FromNodeID_IsPerCallSite_NotPerFile(t *testing.T) {
	// Regression test for the original per-file bug: a single file with
	// TWO functions, each calling a different target, must produce edges
	// originating from EACH function respectively — not both edges
	// originating from whichever function happened to be resolved first.
	table := NewSymbolTable()
	table.Index(astparse.FileCaptures{
		FilePath: "targets.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "TargetA", Kind: astparse.KindFunction},
			{Name: "TargetB", Kind: astparse.KindFunction},
		},
	})
	table.Finalize(nil)

	fc := astparse.FileCaptures{
		FilePath: "callers.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "CallerOne", Kind: astparse.KindFunction, StartLine: 1, EndLine: 5},
			{Name: "CallerTwo", Kind: astparse.KindFunction, StartLine: 10, EndLine: 15},
		},
		Calls: []astparse.CallCapture{
			{Kind: astparse.CallDirect, DirectName: "TargetA", SiteLine: 3},  // inside CallerOne
			{Kind: astparse.CallDirect, DirectName: "TargetB", SiteLine: 12}, // inside CallerTwo
		},
	}

	resolver := NewResolver(table)
	edges, _ := resolver.ResolveEdges(fc)
	require.Len(t, edges, 2)

	byTarget := make(map[NodeID]Edge)
	for _, e := range edges {
		byTarget[e.To] = e
	}

	callerOneID := computeNodeID("callers.go", "CallerOne")
	callerTwoID := computeNodeID("callers.go", "CallerTwo")
	targetAID := computeNodeID("targets.go", "TargetA")
	targetBID := computeNodeID("targets.go", "TargetB")

	assert.Equal(t, callerOneID, byTarget[targetAID].From)
	assert.Equal(t, callerTwoID, byTarget[targetBID].From)
	assert.NotEqual(t, byTarget[targetAID].From, byTarget[targetBID].From,
		"the two calls must be attributed to their OWN enclosing functions, not collapsed to one")
}

func TestResolver_R4_InterfaceDispatch_EmitsOneLowConfidenceEdgePerImplementer(t *testing.T) {
	table := NewSymbolTable()

	table.Index(astparse.FileCaptures{
		FilePath: "notifiers.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "Notify", Kind: astparse.KindMethod, QualifiedHint: "EmailNotifier"},
			{Name: "Notify", Kind: astparse.KindMethod, QualifiedHint: "SMSNotifier"},
		},
	})
	table.Finalize(map[string][]string{"Notifier": {"Notify"}})

	fc := astparse.FileCaptures{
		FilePath: "order_service.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "Send", Kind: astparse.KindFunction, StartLine: 1, EndLine: 10},
		},
		Calls: []astparse.CallCapture{
			{Kind: astparse.CallSelector, OperandName: "n", MethodName: "Notify", SiteLine: 5},
		},
		TypeHints: []astparse.TypeHintCapture{
			{VarName: "n", ResolvedType: "Notifier", InferenceSource: astparse.InferFromParamType},
		},
	}

	resolver := NewResolver(table)
	edges, unresolved := resolver.ResolveEdges(fc)

	require.Len(t, edges, 2, "expected exactly one edge per interface implementer, never a single guessed target")
	assert.Equal(t, 0, unresolved)

	for _, e := range edges {
		assert.Equal(t, ConfidenceInterfaceDispatch, e.Confidence)
		assert.Contains(t, e.Tags, "interface_dispatch")
	}

	targets := make([]NodeID, len(edges))
	for i, e := range edges {
		targets[i] = e.To
	}
	assert.ElementsMatch(t, targets, []NodeID{
		computeNodeID("notifiers.go", "EmailNotifier.Notify"),
		computeNodeID("notifiers.go", "SMSNotifier.Notify"),
	})
}

func TestResolver_P2_SelfCall_ResolvesAgainstEnclosingClass(t *testing.T) {
	table := NewSymbolTable()
	table.Index(astparse.FileCaptures{
		FilePath: "svc.py",
		Symbols: []astparse.SymbolCapture{
			{Name: "reserve", Kind: astparse.KindMethod, QualifiedHint: "InventoryService"},
		},
	})
	table.Finalize(nil)

	fc := astparse.FileCaptures{
		FilePath: "order_service.py",
		Symbols: []astparse.SymbolCapture{
			{Name: "reserve_stock", Kind: astparse.KindMethod, QualifiedHint: "OrderService", StartLine: 1, EndLine: 5},
		},
		Calls: []astparse.CallCapture{
			{Kind: astparse.CallSelector, OperandName: "self", MethodName: "reserve", SiteLine: 3},
		},
	}

	// Note: rule P2 in resolveSelectorCall resolves self.X() against the
	// enclosing symbol's OWN QualifiedHint, so this test actually exercises
	// the "self call resolves within the SAME class" case correctly only
	// if InventoryService IS the enclosing class. Re-shape: enclosing class
	// must match the receiver type holding "reserve".
	fc.Symbols[0].QualifiedHint = "InventoryService"

	resolver := NewResolver(table)
	edges, unresolved := resolver.ResolveEdges(fc)

	require.Len(t, edges, 1)
	assert.Equal(t, ConfidenceSelfCall, edges[0].Confidence)
	assert.Equal(t, 0, unresolved)
}

func TestResolver_P5_DuckTypeAmbiguity_NoTypeHintAtAll(t *testing.T) {
	table := NewSymbolTable()

	// Two UNRELATED classes both happen to define a method named "process" —
	// classic Python duck-typing ambiguity, no type hint available anywhere.
	table.Index(astparse.FileCaptures{
		FilePath: "handlers.py",
		Symbols: []astparse.SymbolCapture{
			{Name: "process", Kind: astparse.KindMethod, QualifiedHint: "PaymentHandler"},
			{Name: "process", Kind: astparse.KindMethod, QualifiedHint: "RefundHandler"},
		},
	})
	table.Finalize(nil)

	fc := astparse.FileCaptures{
		FilePath: "worker.py",
		Symbols: []astparse.SymbolCapture{
			{Name: "run", Kind: astparse.KindFunction, StartLine: 1, EndLine: 5},
		},
		Calls: []astparse.CallCapture{
			{Kind: astparse.CallSelector, OperandName: "handler", MethodName: "process", SiteLine: 3},
		},
		// deliberately NO TypeHintCapture for "handler" — this is the
		// exact P5 scenario: total absence of type evidence.
	}

	resolver := NewResolver(table)
	edges, unresolved := resolver.ResolveEdges(fc)

	require.Len(t, edges, 2)
	assert.Equal(t, 0, unresolved)
	for _, e := range edges {
		assert.Equal(t, ConfidenceDuckTypeAmbiguous, e.Confidence)
		assert.Contains(t, e.Tags, "ambiguous_duck_type")
	}
}

func TestResolver_R7_UnresolvableCall_NeverFabricatesAnEdge(t *testing.T) {
	table := NewSymbolTable() // empty — nothing indexed at all
	table.Finalize(nil)

	fc := astparse.FileCaptures{
		FilePath: "orphan.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "Caller", Kind: astparse.KindFunction, StartLine: 1, EndLine: 5},
		},
		Calls: []astparse.CallCapture{
			{Kind: astparse.CallDirect, DirectName: "TotallyUnknownFunction", SiteLine: 3},
		},
	}

	resolver := NewResolver(table)
	edges, unresolved := resolver.ResolveEdges(fc)

	assert.Empty(t, edges, "R7/P7: an unresolvable call must produce ZERO edges, never a guess")
	assert.Equal(t, 1, unresolved, "the miss must be counted, not silently swallowed")
}

func TestResolver_R6_ReflectiveCall_EmitsLowestConfidenceMarkerEdge(t *testing.T) {
	table := NewSymbolTable()
	table.Finalize(nil)

	fc := astparse.FileCaptures{
		FilePath: "dynamic.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "Dispatch", Kind: astparse.KindFunction, StartLine: 1, EndLine: 5},
		},
		Calls: []astparse.CallCapture{
			{Kind: astparse.CallDynamic, IsReflective: true, SiteLine: 3},
		},
	}

	resolver := NewResolver(table)
	edges, unresolved := resolver.ResolveEdges(fc)

	require.Len(t, edges, 1)
	assert.Equal(t, ConfidenceReflective, edges[0].Confidence)
	assert.Equal(t, unresolvedSentinelID, edges[0].To)
	assert.Contains(t, edges[0].Tags, "reflection")
	assert.Equal(t, 0, unresolved, "reflective calls get a MARKER edge, not a silent miss — distinct from R7")
}

func TestResolver_P6_GetattrLiteral_ResolvesMethodNameDespiteUnknownType(t *testing.T) {
	table := NewSymbolTable()
	table.Index(astparse.FileCaptures{
		FilePath: "handlers.py",
		Symbols: []astparse.SymbolCapture{
			{Name: "handle", Kind: astparse.KindMethod, QualifiedHint: "EventHandler"},
		},
	})
	table.Finalize(nil)

	fc := astparse.FileCaptures{
		FilePath: "dispatcher.py",
		Symbols: []astparse.SymbolCapture{
			{Name: "dispatch", Kind: astparse.KindFunction, StartLine: 1, EndLine: 5},
		},
		Calls: []astparse.CallCapture{
			{Kind: astparse.CallDynamic, LiteralAttr: "handle", SiteLine: 3},
		},
	}

	resolver := NewResolver(table)
	edges, unresolved := resolver.ResolveEdges(fc)

	require.Len(t, edges, 1)
	assert.Equal(t, ConfidenceGetattrLiteral, edges[0].Confidence)
	assert.Contains(t, edges[0].Tags, "getattr_literal")
	assert.Equal(t, 0, unresolved)
}

func TestResolveEdges_IsDeterministic_AcrossRepeatedRuns(t *testing.T) {
	table := NewSymbolTable()
	table.Index(astparse.FileCaptures{
		FilePath: "svc.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "Create", Kind: astparse.KindMethod, QualifiedHint: "OrderService"},
		},
	})
	table.Finalize(nil)

	fc := astparse.FileCaptures{
		FilePath: "caller.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "Handle", Kind: astparse.KindFunction, StartLine: 1, EndLine: 10},
		},
		Calls: []astparse.CallCapture{
			{Kind: astparse.CallSelector, OperandName: "svc", MethodName: "Create", SiteLine: 5},
		},
		TypeHints: []astparse.TypeHintCapture{
			{VarName: "svc", ResolvedType: "OrderService", InferenceSource: astparse.InferFromConstructor},
		},
	}

	resolver := NewResolver(table)
	edgesA, _ := resolver.ResolveEdges(fc)
	edgesB, _ := resolver.ResolveEdges(fc)

	assert.Equal(t, edgesA, edgesB, "identical inputs must produce byte-identical output, every time")
}

func TestResolver_CallOutsideAnySymbol_FallsBackToFileLevelNode(t *testing.T) {
	// A call at a line not covered by ANY SymbolCapture range (e.g. a
	// package-level statement) must not crash or be silently dropped —
	// it attributes to the synthetic file-level node instead.
	table := NewSymbolTable()
	table.Index(astparse.FileCaptures{
		FilePath: "utils.go",
		Symbols: []astparse.SymbolCapture{
			{Name: "Init", Kind: astparse.KindFunction},
		},
	})
	table.Finalize(nil)

	fc := astparse.FileCaptures{
		FilePath: "main.go",
		Symbols:  nil, // no enclosing symbol registered at all
		Calls: []astparse.CallCapture{
			{Kind: astparse.CallDirect, DirectName: "Init", SiteLine: 1},
		},
	}

	resolver := NewResolver(table)
	edges, unresolved := resolver.ResolveEdges(fc)

	require.Len(t, edges, 1)
	assert.Equal(t, 0, unresolved)
	assert.Equal(t, computeNodeID("main.go", "<file-level>"), edges[0].From)
}