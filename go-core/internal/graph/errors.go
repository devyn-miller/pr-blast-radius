// go-core/internal/graph/errors.go
package graph

import "fmt"

// UnresolvedSymbolError indicates a lookup by NodeID found nothing in the
// graph — used sparingly, since most of this package's design goal is to
// avoid needing sharp errors at all (missing symbols are usually handled
// via the "ok bool" pattern instead, e.g. Graph.Nodes[id] lookups).
// Reserved for cases where a caller explicitly requires the symbol to exist.
type UnresolvedSymbolError struct {
	SymbolID NodeID
}

func (e *UnresolvedSymbolError) Error() string {
	return fmt.Sprintf("graph: no node found for symbol_id %q", e.SymbolID)
}
