package astparse

// Language identifies which tree-sitter grammar and .scm query set applies.
type Language string

const (
	LangGo         Language = "go"
	LangPython     Language = "python"
	LangTypeScript Language = "typescript"
)

// SymbolKind classifies a captured definition. Mirrors the Node.kind
// enum in the shared Graph JSON schema exactly — do not diverge.
type SymbolKind string

const (
	KindFunction  SymbolKind = "Function"
	KindMethod    SymbolKind = "Method"
	KindClass     SymbolKind = "Class"
	KindInterface SymbolKind = "Interface"
	// KindModule is the synthetic per-file node for calls made at module
	// level, outside any function or class (e.g. Python service wiring).
	KindModule SymbolKind = "Module"
)

// CallKind classifies HOW a call site was written syntactically. This is
// raw shape information only — resolution and confidence-scoring happen
// one layer up, in package `graph`.
type CallKind string

const (
	CallDirect      CallKind = "Direct"
	CallSelector    CallKind = "Selector"
	CallChained     CallKind = "Chained"
	CallPassedByRef CallKind = "PassedByRef"
	CallDynamic     CallKind = "Dynamic"
)

// InferenceSource records which local heuristic (if any) supplied a
// variable's type for use in resolving Selector/Chained calls.
type InferenceSource string

const (
	InferFromParamType   InferenceSource = "ParamType"
	InferFromConstructor InferenceSource = "Constructor"
	InferFromVarDecl     InferenceSource = "VarDecl"
	InferFromSelfParam   InferenceSource = "SelfParam"
)

// SymbolCapture is one function/method/class/interface found in a file.
// QualifiedHint carries partial context (e.g. a Go method's receiver type,
// or a Python method's enclosing class name) that package `graph` needs to
// build a fully-qualified Node.ID.
type SymbolCapture struct {
	Name          string
	QualifiedHint string
	Kind          SymbolKind
	StartByte     uint32
	EndByte       uint32
	StartLine     int
	EndLine       int
	Exported      bool
	DecoratorTags []string
}

// InterfaceCapture records one interface's declared method set. Go-only —
// PythonParser never populates FileCaptures.Interfaces, and that emptiness
// is itself correct information (Python has no static interface construct),
// not a missing feature.
type InterfaceCapture struct {
	Name    string
	Methods []string
}

// ImportCapture is one import statement/spec found in a file.
type ImportCapture struct {
	ModulePath   string
	Alias        string
	IsDynamic    bool
	LiteralValue string
}

// CallCapture is one call expression found in a file, in its raw,
// UN-resolved form.
type CallCapture struct {
	Kind         CallKind
	DirectName   string
	OperandName  string
	MethodName   string
	OperandType  string
	SiteLine     int
	IsReflective bool
	LiteralAttr  string
}

// TypeHintCapture is one piece of local, syntax-visible type evidence
// used by package `graph`'s Resolver to upgrade a CallCapture's confidence.
type TypeHintCapture struct {
	VarName         string
	ResolvedType    string
	InferenceSource InferenceSource
}

// FileCaptures is the complete Phase A output for a single file — the
// unit-testable boundary of this entire package.
type FileCaptures struct {
	FilePath   string
	Language   Language
	ContentSHA string
	Symbols    []SymbolCapture
	Imports    []ImportCapture
	Calls      []CallCapture
	TypeHints  []TypeHintCapture
	Interfaces []InterfaceCapture
}
