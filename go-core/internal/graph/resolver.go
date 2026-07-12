package graph

import "github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"

// Confidence tiers — shared constants so every resolution rule cites the
// SAME literal value, and so a future retuning is a one-line change here.
const (
	ConfidenceStaticDirect      float32 = 1.0  // R1/P1: direct, unambiguous
	ConfidenceStaticSelector    float32 = 1.0  // R2: cross-package selector, statically resolved
	ConfidenceSelfCall          float32 = 0.95 // P2: self.method() in enclosing class
	ConfidenceLocalInference    float32 = 0.9  // R3/P3: type-hinted or param-typed receiver
	ConfidenceConstructorTrack  float32 = 0.8  // P4: same-scope constructor-tracked variable
	ConfidenceInterfaceDispatch float32 = 0.4  // R4: true dynamic dispatch, per implementer
	ConfidenceDuckTypeAmbiguous float32 = 0.3  // P5: multiple plausible untyped candidates
	ConfidenceCallbackRef       float32 = 0.5  // R5: function passed by value, not directly invoked here
	ConfidenceGetattrLiteral    float32 = 0.4  // P6: getattr(obj, "literal_name")
	ConfidenceReflective        float32 = 0.1  // R6: reflect.* / getattr-on-literal
)

// unresolvedSentinelID is a well-known synthetic NodeID representing "some
// unknown target reached via reflection." graph.BuildGraph inserts exactly
// one synthetic Node with this ID (kind: "Unknown") so downstream consumers
// never encounter a dangling edge to a nonexistent node.
const unresolvedSentinelID NodeID = "sentinel:unresolved-reflective-target"

// Resolver turns one file's raw CallCaptures into confidence-scored Edges,
// using a SymbolTable that has ALREADY been fully indexed (pass 1) and
// finalized across the whole repo. ResolveEdges is a pure function of
// (fc, its own captures) against the table — same inputs, same outputs,
// every time.
type Resolver struct {
	table *SymbolTable
}

func NewResolver(table *SymbolTable) *Resolver {
	return &Resolver{table: table}
}

// ResolveEdges implements the rule table (R1-R7/P1-P8). fromNodeID is
// computed PER CALL SITE, not once per file — a file typically contains
// many functions, each with its own calls, and each call's edge must
// originate from the specific enclosing symbol that contains it, found
// via findEnclosingSymbol's line-range containment check.
func (r *Resolver) ResolveEdges(fc astparse.FileCaptures) ([]Edge, int) {
	localTypes := r.buildLocalTypeMap(fc)

	var edges []Edge
	unresolvedCount := 0

	for _, call := range fc.Calls {
		enclosing := findEnclosingSymbol(fc.Symbols, call.SiteLine)

		fromNodeID := r.fileLevelNodeID(fc) // fallback: calls outside any indexed symbol range
		enclosingHint := ""
		if enclosing != nil {
			qualified := buildQualifiedName(enclosing.Name, enclosing.QualifiedHint)
			fromNodeID = computeNodeID(fc.FilePath, qualified)
			enclosingHint = enclosing.QualifiedHint
		}

		switch call.Kind {

		case astparse.CallDirect:
			resolved, isAmbiguous := r.appendDirectCall(fc, call, fromNodeID)
			edges = append(edges, resolved...)
			if len(resolved) == 0 && !isAmbiguous {
				unresolvedCount++ // R7/P7: truly no match — logged, never fabricated
			}

		case astparse.CallSelector, astparse.CallChained:
			resolved := r.resolveSelectorCall(fc, call, fromNodeID, enclosingHint, localTypes)
			if len(resolved) == 0 {
				unresolvedCount++
			}
			edges = append(edges, resolved...)

		case astparse.CallPassedByRef:
			edges = append(edges, r.appendCallbackRef(fc, call, fromNodeID)...)

		case astparse.CallDynamic:
			edges = append(edges, r.resolveDynamicCall(fc, call, fromNodeID)...)
		}
	}

	return edges, unresolvedCount
}

// findEnclosingSymbol returns the SMALLEST symbol range containing line —
// "smallest" is the correct general rule for nested definitions and costs
// nothing extra to implement. Shared in spirit with gitdiff's hunk-mapping
// concern, but kept local here since Resolver's line coordinate space
// (raw capture line numbers) and gitdiff's (diff hunk line numbers) are
// used in different contexts even though the containment logic is the same shape.
func findEnclosingSymbol(symbols []astparse.SymbolCapture, line int) *astparse.SymbolCapture {
	var best *astparse.SymbolCapture
	bestSpan := -1
	for i := range symbols {
		s := &symbols[i]
		if line < s.StartLine || line > s.EndLine {
			continue
		}
		span := s.EndLine - s.StartLine
		if best == nil || span < bestSpan {
			best, bestSpan = s, span
		}
	}
	return best
}

// fileLevelNodeID is used only when a call occurs outside any indexed
// symbol range (e.g. a package-level var initializer, or a module-level
// statement in Python). A stable, per-file synthetic node — never a guess
// about WHERE the call logically belongs, just an honest label for
// "top of this file."
func (r *Resolver) fileLevelNodeID(fc astparse.FileCaptures) NodeID {
	return computeNodeID(fc.FilePath, "<file-level>")
}

// appendDirectCall handles R1/P1 (unambiguous direct call) and the
// ambiguous-bare-name case (same name defined in multiple places with no
// qualifying context) — returns the edges AND whether the ambiguous branch
// fired, so the caller can distinguish "ambiguous, edges emitted" from
// "truly unresolved, count it."
func (r *Resolver) appendDirectCall(fc astparse.FileCaptures, call astparse.CallCapture, fromNodeID NodeID) ([]Edge, bool) {
	candidates := r.table.CandidatesByQualifiedName(call.DirectName)

	if len(candidates) == 1 {
		return []Edge{{
			From: fromNodeID, To: candidates[0].nodeID, Type: EdgeCalls,
			Confidence: ConfidenceStaticDirect,
			Site:       CallSite{File: fc.FilePath, Line: call.SiteLine},
		}}, false
	}

	if len(candidates) > 1 {
		var edges []Edge
		for _, cand := range candidates {
			edges = append(edges, Edge{
				From: fromNodeID, To: cand.nodeID, Type: EdgeCalls,
				Confidence: ConfidenceDuckTypeAmbiguous,
				Tags:       []string{"ambiguous_duck_type"},
				Site:       CallSite{File: fc.FilePath, Line: call.SiteLine},
			})
		}
		return edges, true
	}

	return nil, false // R7/P7 — no match at all
}

// appendCallbackRef handles R5: a function passed as a value (callback),
// not directly invoked here — we know a dependency exists, but not
// necessarily when/whether it's invoked.
func (r *Resolver) appendCallbackRef(fc astparse.FileCaptures, call astparse.CallCapture, fromNodeID NodeID) []Edge {
	candidates := r.table.CandidatesByQualifiedName(call.DirectName)
	if len(candidates) != 1 {
		return nil // ambiguous or unresolvable callback references are deliberately not guessed at
	}
	return []Edge{{
		From: fromNodeID, To: candidates[0].nodeID, Type: EdgeCalls,
		Confidence: ConfidenceCallbackRef,
		Tags:       []string{"callback_reference"},
		Site:       CallSite{File: fc.FilePath, Line: call.SiteLine},
	}}
}

// resolveSelectorCall handles R2/R3/R4/P2/P3/P4/P5 — the largest branch,
// since selector/attribute calls are the most common call shape in both
// languages. enclosingHint is the qualified name (receiver type / class)
// of the function CONTAINING this call site, passed in directly by
// ResolveEdges rather than looked up via a stub — this is what makes
// self.method()/P2 resolution correct per-call instead of per-file.
func (r *Resolver) resolveSelectorCall(
	fc astparse.FileCaptures, call astparse.CallCapture, fromNodeID NodeID,
	enclosingHint string, localTypes map[string]typeHintResult,
) []Edge {
	// P2: self.method() — resolves against the enclosing class directly.
	if call.OperandName == "self" && enclosingHint != "" {
		for _, m := range r.table.MethodsOnReceiverType(enclosingHint) {
			if m.name == call.MethodName {
				return []Edge{{
					From: fromNodeID, To: m.nodeID, Type: EdgeCalls,
					Confidence: ConfidenceSelfCall,
					Site:       CallSite{File: fc.FilePath, Line: call.SiteLine},
				}}
			}
		}
	}

	// R2: module-alias selector — pkg.Foo() where pkg is a known import.
	if modulePath, ok := r.table.ResolveImportAlias(fc.FilePath, call.OperandName); ok {
		qualified := modulePath + "." + call.MethodName
		if candidates := r.table.CandidatesByQualifiedName(qualified); len(candidates) == 1 {
			return []Edge{{
				From: fromNodeID, To: candidates[0].nodeID, Type: EdgeCalls,
				Confidence: ConfidenceStaticSelector,
				Site:       CallSite{File: fc.FilePath, Line: call.SiteLine},
			}}
		}
	}

	// R3/P3/P4: operand's type known via local inference.
	if hint, ok := localTypes[call.OperandName]; ok {
		if methods, isInterface := r.table.interfaces[hint.resolvedType]; isInterface && methods != nil {
			// R4: the resolved "type" is an INTERFACE, not a concrete
			// struct -> true dynamic dispatch, one low-confidence edge
			// per implementer, never a single guessed target.
			var edges []Edge
			for _, implType := range r.table.ImplementersOf(hint.resolvedType) {
				for _, m := range r.table.MethodsOnReceiverType(implType) {
					if m.name == call.MethodName {
						edges = append(edges, Edge{
							From: fromNodeID, To: m.nodeID, Type: EdgeCalls,
							Confidence: ConfidenceInterfaceDispatch,
							Tags:       []string{"interface_dispatch"},
							Site:       CallSite{File: fc.FilePath, Line: call.SiteLine},
						})
					}
				}
			}
			return edges
		}

		for _, m := range r.table.MethodsOnReceiverType(hint.resolvedType) {
			if m.name == call.MethodName {
				confidence := ConfidenceLocalInference
				if hint.source == astparse.InferFromConstructor {
					confidence = ConfidenceConstructorTrack
				}
				return []Edge{{
					From: fromNodeID, To: m.nodeID, Type: EdgeCalls,
					Confidence: confidence,
					Site:       CallSite{File: fc.FilePath, Line: call.SiteLine},
				}}
			}
		}
	}

	// P5: no type evidence at all — every symbol anywhere with this
	// method name gets a low-confidence edge, never a single guess.
	var duckEdges []Edge
	seen := make(map[NodeID]bool)
	for _, entries := range r.table.byReceiverType {
		for _, e := range entries {
			if e.name == call.MethodName && !seen[e.nodeID] {
				seen[e.nodeID] = true
				duckEdges = append(duckEdges, Edge{
					From: fromNodeID, To: e.nodeID, Type: EdgeCalls,
					Confidence: ConfidenceDuckTypeAmbiguous,
					Tags:       []string{"ambiguous_duck_type"},
					Site:       CallSite{File: fc.FilePath, Line: call.SiteLine},
				})
			}
		}
	}
	return duckEdges
}

// resolveDynamicCall handles R6/P6/P8: reflection and getattr-based dispatch.
func (r *Resolver) resolveDynamicCall(fc astparse.FileCaptures, call astparse.CallCapture, fromNodeID NodeID) []Edge {
	if call.LiteralAttr != "" {
		// P6: getattr(obj, "literal_name") — method name IS known, even
		// though obj's type is not.
		var edges []Edge
		for _, entries := range r.table.byReceiverType {
			for _, e := range entries {
				if e.name == call.LiteralAttr {
					edges = append(edges, Edge{
						From: fromNodeID, To: e.nodeID, Type: EdgeCalls,
						Confidence: ConfidenceGetattrLiteral,
						Tags:       []string{"getattr_literal"},
						Site:       CallSite{File: fc.FilePath, Line: call.SiteLine},
					})
				}
			}
		}
		return edges
	}

	if call.IsReflective {
		// R6: reflect.MethodByName/.Call, or getattr with a non-literal
		// name — truly unresolvable target. We still emit a marker edge
		// so this shows up as a flagged risk rather than disappearing.
		return []Edge{{
			From: fromNodeID, To: unresolvedSentinelID, Type: EdgeCalls,
			Confidence: ConfidenceReflective,
			Tags:       []string{"reflection", "unresolved_target"},
			Site:       CallSite{File: fc.FilePath, Line: call.SiteLine},
		}}
	}

	return nil
}

// typeHintResult bundles a resolved type with which inference rule
// produced it, since that determines R3 vs P4's confidence split.
type typeHintResult struct {
	resolvedType string
	source       astparse.InferenceSource
}

// buildLocalTypeMap flattens a file's TypeHintCaptures into a lookup by
// variable name, scoped to the whole file — a documented simplification
// (true per-function scoping would require tracking block boundaries,
// which is the "single-file, syntax-driven, not full data-flow analysis"
// limitation stated in the project README).
func (r *Resolver) buildLocalTypeMap(fc astparse.FileCaptures) map[string]typeHintResult {
	m := make(map[string]typeHintResult, len(fc.TypeHints))
	for _, h := range fc.TypeHints {
		if h.ResolvedType == "" {
			continue // e.g. InferFromSelfParam hints carry no ResolvedType by design
		}
		m[h.VarName] = typeHintResult{resolvedType: h.ResolvedType, source: h.InferenceSource}
	}
	return m
}