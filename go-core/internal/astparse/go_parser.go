package astparse

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
)

// GoParser is the Parser implementation for Go source files. It owns one
// compiled sitter.Language and four pre-compiled queries (symbols, imports,
// calls, typehints), all loaded once at construction time via loadQuery
// rather than re-reading/re-compiling per file.
type GoParser struct {
	lang           *sitter.Language
	symbolsQuery   *sitter.Query
	importsQuery   *sitter.Query
	callsQuery     *sitter.Query
	typeHintsQuery *sitter.Query
}

// NewGoParser compiles all four Go queries against the embedded .scm files.
// Returns an error (never panics) if a query fails to compile.
func NewGoParser() (*GoParser, error) {
	lang := golang.GetLanguage()

	compiled := make(map[string]*sitter.Query, 4)
	for _, name := range []string{"symbols", "imports", "calls", "typehints"} {
		src, err := loadQuery(LangGo, name)
		if err != nil {
			return nil, err
		}
		q, err := sitter.NewQuery([]byte(src), lang)
		if err != nil {
			return nil, fmt.Errorf("astparse: compiling go/%s.scm: %w", name, err)
		}
		compiled[name] = q
	}

	return &GoParser{
		lang:           lang,
		symbolsQuery:   compiled["symbols"],
		importsQuery:   compiled["imports"],
		callsQuery:     compiled["calls"],
		typeHintsQuery: compiled["typehints"],
	}, nil
}

func (p *GoParser) Language() Language { return LangGo }

// Parse runs Phase A capture only: parse to a tree once, then run each of
// the four queries against that same tree. No symbol-table lookups, no
// cross-file resolution.
func (p *GoParser) Parse(ctx context.Context, filePath string, source []byte) (FileCaptures, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(p.lang)

	tree, err := parser.ParseCtx(ctx, nil, source)
	if err != nil {
		return FileCaptures{}, fmt.Errorf("astparse: tree-sitter parse failed for %s: %w", filePath, err)
	}
	defer tree.Close()

	root := tree.RootNode()

	symbols, interfaces, err := p.extractSymbols(root, source)
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
		Symbols:    symbols,
		Imports:    imports,
		Calls:      calls,
		TypeHints:  typeHints,
		Interfaces: interfaces,
	}, nil
}

// --- Symbols ---------------------------------------------------------

// extractSymbols returns BOTH the flat symbol list (functions, methods,
// classes, and interfaces themselves as KindInterface symbols) AND a
// separate, dedicated []InterfaceCapture carrying each interface's full
// declared method set — package `graph`'s SymbolTable.Finalize needs the
// latter to build the interface-implementer index (rule R4), and that
// data has no natural home on SymbolCapture itself (a symbol has ONE
// QualifiedHint slot, not a method-set list).
func (p *GoParser) extractSymbols(root *sitter.Node, source []byte) ([]SymbolCapture, []InterfaceCapture, error) {
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(p.symbolsQuery, root)

	var symbols []SymbolCapture
	var interfaces []InterfaceCapture

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		m = qc.FilterPredicates(m, source)

		var (
			name         string
			receiverType string
			kind         SymbolKind
			defNode      *sitter.Node
			ifaceName    string
			ifaceMethods []string
		)

		for _, c := range m.Captures {
			capName := p.symbolsQuery.CaptureNameForId(c.Index)
			text := c.Node.Content(source)

			switch capName {
			case "function.name":
				name, kind = text, KindFunction
				defNode = firstNonNil(defNode, c.Node.Parent())
			case "method.name":
				name, kind = text, KindMethod
				defNode = firstNonNil(defNode, c.Node.Parent())
			case "method.receiver_type":
				receiverType = text
			case "type.name":
				name, kind = text, KindClass
				defNode = firstNonNil(defNode, walkUpToTypeDeclaration(c.Node))
			case "interface.name":
				ifaceName = text
				name, kind = text, KindInterface
				defNode = firstNonNil(defNode, walkUpToTypeDeclaration(c.Node))
			case "interface.method_name":
				// (method_elem ...)* means one interface match can carry
				// MULTIPLE of these captures — one per declared method —
				// so we accumulate rather than overwrite.
				ifaceMethods = append(ifaceMethods, text)
			}
		}

		if ifaceName != "" {
			interfaces = append(interfaces, InterfaceCapture{Name: ifaceName, Methods: ifaceMethods})
		}

		if name == "" || defNode == nil {
			continue // incomplete match — skip rather than emit a partial/garbage symbol
		}

		symbols = append(symbols, SymbolCapture{
			Name:          name,
			QualifiedHint: receiverType,
			Kind:          kind,
			StartByte:     defNode.StartByte(),
			EndByte:       defNode.EndByte(),
			StartLine:     int(defNode.StartPoint().Row) + 1,
			EndLine:       int(defNode.EndPoint().Row) + 1,
			Exported:      isExported(name),
		})
	}

	return symbols, interfaces, nil
}

// walkUpToTypeDeclaration climbs from a captured type_identifier up to the
// enclosing type_declaration node, since that's the node whose byte range
// represents the full "type X struct {...}" definition, not just the name.
func walkUpToTypeDeclaration(n *sitter.Node) *sitter.Node {
	for cur := n; cur != nil; cur = cur.Parent() {
		if cur.Type() == "type_declaration" {
			return cur
		}
	}
	return n
}

func firstNonNil(a, b *sitter.Node) *sitter.Node {
	if a != nil {
		return a
	}
	return b
}

// isExported applies Go's convention directly: a name is exported iff its
// first rune is uppercase — a language rule, not a heuristic.
func isExported(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper([]rune(name)[0])
}

// --- Imports -----------------------------------------------------------

func (p *GoParser) extractImports(root *sitter.Node, source []byte) ([]ImportCapture, error) {
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

		var imp ImportCapture
		for _, c := range m.Captures {
			switch p.importsQuery.CaptureNameForId(c.Index) {
			case "import.alias":
				imp.Alias = c.Node.Content(source)
			case "import.path":
				imp.ModulePath = trimQuotes(c.Node.Content(source))
			}
		}
		if imp.ModulePath != "" {
			imports = append(imports, imp)
		}
	}

	return imports, nil
}

func trimQuotes(s string) string {
	return strings.Trim(s, `"'`)
}

// --- Calls ---------------------------------------------------------------

func (p *GoParser) extractCalls(root *sitter.Node, source []byte) ([]CallCapture, error) {
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
			case "reflect.method":
				call.Kind = CallDynamic
				call.IsReflective = true
				call.MethodName = text
				call.SiteLine = line
				matched = true
			}
		}

		if matched {
			calls = append(calls, call)
		}
	}

	return calls, nil
}

// --- Type hints (local inference evidence) --------------------------------

func (p *GoParser) extractTypeHints(root *sitter.Node, source []byte) ([]TypeHintCapture, error) {
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
			varName, varType, ctorName string
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
			}
		}

		switch {
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
