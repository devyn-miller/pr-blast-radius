package astparse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

// Parser performs Phase A syntactic capture for exactly one language.
// A Parser implementation owns: (1) the compiled tree-sitter grammar,
// (2) the three embedded .scm query sets (symbols/imports/calls), and
// (3) the tree-sitter query execution + capture-to-struct mapping.
//
// A Parser MUST NOT perform symbol resolution, type inference beyond
// what's syntactically visible in a single file, or any cross-file
// lookup. That is package `graph`'s responsibility (Phase B). This
// boundary is what keeps Parser implementations independently
// unit-testable against single fixture files.
type Parser interface {
	Language() Language
	Parse(ctx context.Context, filePath string, source []byte) (FileCaptures, error)
}

// Registry dispatches to the correct Parser by file extension. It is the
// only thing outside this package that other code should depend on —
// never a concrete *GoParser or *PythonParser directly. This indirection
// is what lets `graph.BuildGraph` iterate over a mixed-language file set
// without language-specific branching anywhere in its own code.
type Registry struct {
	parsers    map[Language]Parser
	extToLang  map[string]Language
}

// NewRegistry builds a Registry from a set of already-constructed Parsers.
// Composition happens at the call site (typically cmd/blastradius's main,
// or a test's setup) — Registry itself never constructs Parsers, keeping
// it trivial to substitute a fake/spy Parser in tests.
func NewRegistry(parsers ...Parser) *Registry {
	r := &Registry{
		parsers: make(map[Language]Parser, len(parsers)),
		extToLang: map[string]Language{
			".go":  LangGo,
			".py":  LangPython,
			".ts":  LangTypeScript,
			".tsx": LangTypeScript,
		},
	}
	for _, p := range parsers {
		r.parsers[p.Language()] = p
	}
	return r
}

// ParserFor returns the Parser responsible for a given file path's
// extension. The bool is false (not an error) for unsupported extensions —
// callers walking a repo tree should skip such files silently, since a
// repo legitimately contains many non-source files.
func (r *Registry) ParserFor(filePath string) (Parser, bool) {
	lang, ok := r.extToLang[filepath.Ext(filePath)]
	if !ok {
		return nil, false
	}
	p, ok := r.parsers[lang]
	return p, ok
}

// ParseFile resolves the correct Parser and invokes it, additionally
// computing ContentSHA here (once, in one place) rather than leaving
// each Parser implementation to remember to do it — this hash is what
// package `cache` keys its invalidation on, so it must never be skipped
// or computed inconsistently between languages.
func (r *Registry) ParseFile(ctx context.Context, filePath string, source []byte) (FileCaptures, error) {
	parser, ok := r.ParserFor(filePath)
	if !ok {
		return FileCaptures{}, &UnsupportedFileError{FilePath: filePath}
	}

	fc, err := parser.Parse(ctx, filePath, source)
	if err != nil {
		return FileCaptures{}, fmt.Errorf("parsing %s: %w", filePath, err)
	}

	fc.FilePath = filePath
	fc.Language = parser.Language()
	fc.ContentSHA = contentHash(source)
	return fc, nil
}

func contentHash(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}