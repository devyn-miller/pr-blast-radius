package astparse

import (
	"context"
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
)

// PythonParser is the Parser implementation for Python source files.
// Structurally identical in shape to GoParser (same four-query pipeline),
// which is the point: this parity is what proves the Parser interface
// generalizes across a statically-typed and a dynamically-typed grammar
// without the Registry or anything above it needing to know the difference.
type PythonParser struct {
	lang           *sitter.Language
	symbolsQuery   *sitter.Query
	importsQuery   *sitter.Query
	callsQuery     *sitter.Query
	typeHintsQuery *sitter.Query
}

func NewPythonParser() (*PythonParser, error) {
	lang := python.GetLanguage()

	compiled := make(map[string]*sitter.Query, 4)
	for _, name := range []string{"symbols", "imports", "calls", "typehints"} {
		src, err := loadQuery(LangPython, name)
		if err != nil {
			return nil, err
		}
		q, err := sitter.NewQuery([]byte(src), lang)
		if err != nil {
			return nil, fmt.Errorf("astparse: compiling python/%s.scm: %w", name, err)
		}
		compiled[name] = q
	}

	return &PythonParser{
		lang:           lang,
		symbolsQuery:   compiled["symbols"],
		importsQuery:   compiled["imports"],
		callsQuery:     compiled["calls"],
		typeHintsQuery: compiled["typehints"],
	}, nil
}

func (p *PythonParser) Language() Language { return LangPython }

func (p *PythonParser) Parse(ctx context.Context, filePath string, source []byte) (FileCaptures, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(p.lang)

	tree, err := parser.ParseCtx(ctx, nil, source)
	if err != nil {
		return FileCaptures{}, fmt.Errorf("astparse: tree-sitter parse failed for %s: %w", filePath, err)
	}
	defer tree.Close()

	root := tree.RootNode()

	symbols, err := p.extractSymbols(root, source)
	if err != nil {
		return FileCaptures{}, err
	}
	imports, err := p.extractImports(root, source)
	if err != nil {
		return FileCaptures{}, err
	}
	calls, err := p.extractCalls(root, source)
	if err != nil {
		return FileCaptures{}, err
	}
	typeHints, err := p.extractTypeHints(root, source)
	if err != nil {
		return FileCaptures{}, err
	}

	return FileCaptures{
		Symbols:   symbols,
		Imports:   imports,
		Calls:     calls,
		TypeHints: typeHints,
	}, nil
}

// --- Symbols -------------------------------------------------------------

// routeTag holds decorator-derived metadata for one decorated function,
// keyed by function name, before being merged onto its SymbolCapture.
// Purely internal — never leaves this file.
type routeTag struct {
	object    string // e.g. "app"
	attribute string // e.g. "post", "get" — the HTTP verb, Flask short-form
	path      string // e.g. "/orders"
}

func (p *PythonParser) extractSymbols(root *sitter.Node, source []byte) ([]SymbolCapture, error) {
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(p.symbolsQuery, root)

	type rangeKey struct{ start, end uint32 }
	bestByRange := make(map[rangeKey]SymbolCapture)
	var order []rangeKey

	routeTags := make(map[string]routeTag)

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		m = qc.FilterPredicates(m, source)

		var (
			name, classOwner string
			kind             SymbolKind
			defNode          *sitter.Node
			rt               routeTag
			isDecoMatch      bool
		)

		for _, c := range m.Captures {
			text := c.Node.Content(source)
			switch p.symbolsQuery.CaptureNameForId(c.Index) {
			case "function.name":
				name, kind = text, KindFunction
				if defNode == nil {
					defNode = c.Node.Parent()
				}
			case "class.name":
				classOwner = text
			case "class.definition":
				kind = KindClass
				defNode = c.Node
			case "method.name":
				name, kind = text, KindMethod
			case "method.definition":
				defNode = c.Node
			case "decorator.object":
				rt.object = text
			case "decorator.attribute":
				rt.attribute = text
			case "decorator.route_path":
				rt.path = trimQuotes(text)
				isDecoMatch = true
			case "decorated.function_name":
				name = text
			}
		}

		if isDecoMatch {
			routeTags[name] = rt
			continue
		}

		// A plain class-pattern match sets kind=KindClass via the
		// class.definition capture, but only class.name carries the
		// actual identifier text — copy it into name here, since this
		// is the ONE place both pieces of information are in scope
		// together.
		if kind == KindClass && name == "" {
			name = classOwner
		}

		qualifiedHint := ""
		if kind == KindMethod {
			qualifiedHint = classOwner
		}

		if name == "" || defNode == nil {
			continue
		}

		key := rangeKey{defNode.StartByte(), defNode.EndByte()}
		candidate := SymbolCapture{
			Name: name, QualifiedHint: qualifiedHint, Kind: kind,
			StartByte: defNode.StartByte(), EndByte: defNode.EndByte(),
			StartLine: int(defNode.StartPoint().Row) + 1,
			EndLine:   int(defNode.EndPoint().Row) + 1,
			Exported:  isPythonPublic(name),
		}

		existing, seen := bestByRange[key]
		if !seen {
			bestByRange[key] = candidate
			order = append(order, key)
		} else if existing.Kind == KindFunction && candidate.Kind == KindMethod {
			// Same underlying node matched BOTH the generic top-level
			// function pattern and the nested-in-class method pattern
			// (queries are unanchored by default) — Method is always
			// the more specific, correct classification.
			bestByRange[key] = candidate
		}
	}

	symbols := make([]SymbolCapture, 0, len(order))
	for _, k := range order {
		symbols = append(symbols, bestByRange[k])
	}

	for i := range symbols {
		if rt, ok := routeTags[symbols[i].Name]; ok && rt.path != "" {
			method := strings.ToUpper(rt.attribute)
			symbols[i].DecoratorTags = append(symbols[i].DecoratorTags,
				fmt.Sprintf("route:%s:%s", method, rt.path))
		}
	}

	return symbols, nil
}

// isPythonPublic applies Python's convention: a name is "public" unless it
// starts with an underscore. Like Go's isExported, this is a direct
// language-convention rule, not a heuristic — safe to hard-code at
// confidence 1.0.
func isPythonPublic(name string) bool {
	return !strings.HasPrefix(name, "_")
}

// --- Imports ---------------------------------------------------------------

func (p *PythonParser) extractImports(root *sitter.Node, source []byte) ([]ImportCapture, error) {
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(p.importsQuery, root)

	var imports []ImportCapture

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		m = qc.FilterPredicates(m, source)

		var (
			imp        ImportCapture
			isDynamic  bool
			hasLiteral bool
		)

		for _, c := range m.Captures {
			text := c.Node.Content(source)
			switch p.importsQuery.CaptureNameForId(c.Index) {
			case "import.module":
				imp.ModulePath = text
			case "import.alias":
				imp.Alias = text
			case "import.symbol":
				// from X import Y — Y is a name pulled from module X, not
				// a separate module path; fold it in for symbol-table use
				// downstream without inventing a new field.
				imp.ModulePath = imp.ModulePath // module already set by import.module in same match
				imp.Alias = text
			case "dyn.obj", "dyn.method":
				isDynamic = true
			case "dyn.literal_module":
				imp.LiteralValue = trimQuotes(text)
				hasLiteral = true
			}
		}

		imp.IsDynamic = isDynamic
		if isDynamic && !hasLiteral {
			// importlib.import_module(some_variable) — module name is
			// unresolvable here. Per P7/P8's integrity rule: we still
			// record the capture (IsDynamic=true, LiteralValue="") so
			// graph.Resolver can count it toward unresolved_call_count
			// rather than silently dropping it.
			imports = append(imports, imp)
			continue
		}

		if imp.ModulePath != "" || imp.LiteralValue != "" {
			imports = append(imports, imp)
		}
	}

	return imports, nil
}

// func trimQuotes(s string) string {
// 	return strings.Trim(s, `"'`)
// }

// --- Calls -------------------------------------------------------------

func (p *PythonParser) extractCalls(root *sitter.Node, source []byte) ([]CallCapture, error) {
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(p.callsQuery, root)

	var calls []CallCapture

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		m = qc.FilterPredicates(m, source)

		var (
			call    CallCapture
			matched bool
		)

		for _, c := range m.Captures {
			line := int(c.Node.StartPoint().Row) + 1
			text := c.Node.Content(source)

			switch p.callsQuery.CaptureNameForId(c.Index) {
			case "call.direct_name":
				call = CallCapture{Kind: CallDirect, DirectName: text, SiteLine: line}
				matched = true
			case "call.operand":
				call.Kind = CallSelector
				call.OperandName = text
				call.SiteLine = line
				matched = true
			case "call.operand_chain":
				call.Kind = CallChained
				call.OperandName = text
				call.SiteLine = line
				matched = true
			case "call.method_name":
				call.MethodName = text
			case "call.passed_as_value":
				call = CallCapture{Kind: CallPassedByRef, DirectName: text, SiteLine: line}
				matched = true
			case "dyn.fn":
				call = CallCapture{Kind: CallDynamic, IsReflective: true, SiteLine: line}
				matched = true
			case "dyn.target":
				call.OperandName = text
			case "dyn.attr_name_literal":
				call.LiteralAttr = trimQuotes(text)
			}
		}

		if matched {
			calls = append(calls, call)
		}
	}

	return calls, nil
}

// --- Type hints ----------------------------------------------------------

func (p *PythonParser) extractTypeHints(root *sitter.Node, source []byte) ([]TypeHintCapture, error) {
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(p.typeHintsQuery, root)

	var hints []TypeHintCapture

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		m = qc.FilterPredicates(m, source)

		var (
			varName, varType, ctorName, selfParam string
		)

		for _, c := range m.Captures {
			text := c.Node.Content(source)
			switch p.typeHintsQuery.CaptureNameForId(c.Index) {
			case "var.name":
				varName = text
			case "var.type":
				varType = text
			case "var.constructor_name":
				ctorName = text
			case "self.param_name":
				selfParam = text
			}
		}

		switch {
		case selfParam != "":
			hints = append(hints, TypeHintCapture{
				VarName: selfParam, InferenceSource: InferFromSelfParam,
			})
		case varName != "" && varType != "":
			hints = append(hints, TypeHintCapture{
				VarName: varName, ResolvedType: varType, InferenceSource: InferFromParamType,
			})
		case varName != "" && ctorName != "":
			hints = append(hints, TypeHintCapture{
				VarName: varName, ResolvedType: ctorName, InferenceSource: InferFromConstructor,
			})
		}
	}

	return hints, nil
}