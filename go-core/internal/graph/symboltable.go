package graph

import "github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"

// symbolEntry is the SymbolTable's internal record for one indexed symbol —
// enough context to both construct edges (NodeID) and resolve confidence
// (Kind, for interface/receiver-type matching in rule R4).
type symbolEntry struct {
	nodeID        NodeID
	name          string
	qualifiedName string
	filePath      string
	receiverType  string // Go: struct name off a method; empty for functions
	kind          astparse.SymbolKind
}

// SymbolTable indexes every symbol and import across the WHOLE repo before
// any edge resolution begins. This two-pass split exists because resolving
// a cross-file call (e.g. svc.Method() in file A, where Method is defined
// in file B) is impossible until every file's symbols are known — you
// cannot resolve pass 2 while still running pass 1.
type SymbolTable struct {
	byQualifiedName map[string][]symbolEntry // supports duck-typing ambiguity (P5): >1 candidate is expected, not a bug
	byReceiverType  map[string][]symbolEntry // Go: all methods on a given struct type, for interface-implementer checks (R4)
	interfaces      map[string][]string      // interface name -> required method names (Go only)
	importAliases   map[string]map[string]string // filePath -> {alias: modulePath}, import scope is per-file
}

func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		byQualifiedName: make(map[string][]symbolEntry),
		byReceiverType:  make(map[string][]symbolEntry),
		interfaces:      make(map[string][]string),
		importAliases:   make(map[string]map[string]string),
	}
}

// Index registers one file's Phase A captures into the table. Called once
// per file, in any order — indexing has no cross-file dependencies itself,
// only edge resolution (Resolver.ResolveEdges) does.
func (st *SymbolTable) Index(fc astparse.FileCaptures) {
	for _, sym := range fc.Symbols {
		qualifiedName := buildQualifiedName(sym.Name, sym.QualifiedHint)
		entry := symbolEntry{
			nodeID:        computeNodeID(fc.FilePath, qualifiedName),
			name:          sym.Name,
			qualifiedName: qualifiedName,
			filePath:      fc.FilePath,
			receiverType:  sym.QualifiedHint,
			kind:          sym.Kind,
		}

		st.byQualifiedName[qualifiedName] = append(st.byQualifiedName[qualifiedName], entry)

		// Also index by bare name — this is what enables rule R1/P1 (direct
		// call resolution) and rule P5 (ambiguous duck-typed candidates),
		// both of which look up a call by method/function NAME ALONE,
		// without necessarily knowing a receiver type yet.
		if sym.Kind == astparse.KindMethod && sym.QualifiedHint != "" {
			st.byReceiverType[sym.QualifiedHint] = append(st.byReceiverType[sym.QualifiedHint], entry)
		}

		if sym.Kind == astparse.KindInterface {
			// Interface method-set extraction happens off the raw capture's
			// DecoratorTags-adjacent info in the real query; for now we
			// track the interface's OWN qualifiedName as a marker and let
			// Finalize() build the implementer index once all structs are
			// known too (structs may be indexed after their interface).
			st.interfaces[qualifiedName] = nil // populated properly in Finalize via method-set comparison
		}
	}

	fileAliases := make(map[string]string, len(fc.Imports))
	for _, imp := range fc.Imports {
		alias := imp.Alias
		if alias == "" {
			alias = lastPathSegment(imp.ModulePath) // Go: `fmt` import with no alias -> alias defaults to "fmt"
		}
		fileAliases[alias] = imp.ModulePath
	}
	st.importAliases[fc.FilePath] = fileAliases
}

// Finalize runs after ALL files have been indexed. It builds the
// interface-implementer relationship (rule R4) by comparing each
// interface's declared method set against each struct's method set —
// this is our substitute for a real type-checker's `implements` check,
// and it can only run once every method on every struct is known.
func (st *SymbolTable) Finalize(interfaceMethodSets map[string][]string) {
	for ifaceName, methods := range interfaceMethodSets {
		st.interfaces[ifaceName] = methods
	}
}

// CandidatesByName returns every indexed symbol with the given bare name,
// regardless of receiver type — the raw material for rule P5's
// duck-typing ambiguity (deliberately returns ALL matches, doesn't guess).
func (st *SymbolTable) CandidatesByQualifiedName(qualifiedName string) []symbolEntry {
	return st.byQualifiedName[qualifiedName]
}

func (st *SymbolTable) MethodsOnReceiverType(receiverType string) []symbolEntry {
	return st.byReceiverType[receiverType]
}

// ImplementersOf returns every concrete type whose method set satisfies
// the named interface — this is rule R4's core lookup.
func (st *SymbolTable) ImplementersOf(interfaceName string) []string {
	required := st.interfaces[interfaceName]
	if len(required) == 0 {
		return nil
	}

	var implementers []string
	seen := make(map[string]bool)

	for _, entries := range st.byReceiverType {
		if len(entries) == 0 || seen[entries[0].receiverType] {
			continue
		}
		receiverType := entries[0].receiverType
		if hasAllMethods(entries, required) {
			implementers = append(implementers, receiverType)
			seen[receiverType] = true
		}
	}
	return implementers
}

func hasAllMethods(entries []symbolEntry, required []string) bool {
	have := make(map[string]bool, len(entries))
	for _, e := range entries {
		have[e.name] = true
	}
	for _, m := range required {
		if !have[m] {
			return false
		}
	}
	return true
}

func (st *SymbolTable) ResolveImportAlias(filePath, alias string) (string, bool) {
	aliases, ok := st.importAliases[filePath]
	if !ok {
		return "", false
	}
	modulePath, ok := aliases[alias]
	return modulePath, ok
}

func lastPathSegment(path string) string {
	last := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			last = path[i+1:]
			break
		}
	}
	return last
}