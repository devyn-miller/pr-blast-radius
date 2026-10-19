package graph

import (
	"crypto/sha1"
	"encoding/hex"
)

// computeNodeID is a pure function: identical (filePath, qualifiedName)
// input always produces an identical NodeID, in this run or any other.
// This determinism is a tested invariant (NFR: "given the same diff +
// same repo state, the graph must be byte-identical across runs") and
// it's also what lets package `cache` key its SQLite rows meaningfully.
func computeNodeID(filePath, qualifiedName string) NodeID {
	h := sha1.New()
	h.Write([]byte(filePath))
	h.Write([]byte{0}) // NUL separator — prevents "a"+"bc" colliding with "ab"+"c"
	h.Write([]byte(qualifiedName))
	return NodeID(hex.EncodeToString(h.Sum(nil)))
}

// buildQualifiedName composes a symbol's fully-qualified name from its
// SymbolCapture, using QualifiedHint (receiver type in Go, enclosing
// class in Python) when present. Centralized here so Go's "Type.Method"
// convention and Python's "Class.method" convention are both handled by
// one function, rather than duplicated logic in SymbolTable.Index.
func buildQualifiedName(name, qualifiedHint string) string {
	if qualifiedHint == "" {
		return name
	}
	return qualifiedHint + "." + name
}
