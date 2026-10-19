package astparse

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeParser is a test double — proves Registry dispatch logic works
// correctly with zero dependency on a real tree-sitter grammar.
type fakeParser struct {
	lang   Language
	result FileCaptures
	err    error
}

func (f *fakeParser) Language() Language { return f.lang }

func (f *fakeParser) Parse(ctx context.Context, filePath string, source []byte) (FileCaptures, error) {
	return f.result, f.err
}

func TestRegistry_DispatchesByExtension(t *testing.T) {
	goParser := &fakeParser{lang: LangGo, result: FileCaptures{
		Symbols: []SymbolCapture{{Name: "CreateOrder", Kind: KindFunction, Exported: true}},
	}}
	pyParser := &fakeParser{lang: LangPython, result: FileCaptures{
		Symbols: []SymbolCapture{{Name: "apply_discount", Kind: KindFunction, Exported: true}},
	}}
	registry := NewRegistry(goParser, pyParser)

	fc, err := registry.ParseFile(context.Background(), "services/order.go", []byte("package services"))
	require.NoError(t, err)
	assert.Equal(t, LangGo, fc.Language)
	assert.Equal(t, "CreateOrder", fc.Symbols[0].Name)

	fc, err = registry.ParseFile(context.Background(), "services/pricing/discount_engine.py", []byte("def apply_discount(): pass"))
	require.NoError(t, err)
	assert.Equal(t, LangPython, fc.Language)
	assert.Equal(t, "apply_discount", fc.Symbols[0].Name)
}

func TestRegistry_UnsupportedExtension_ReturnsTypedError(t *testing.T) {
	registry := NewRegistry() // no parsers registered at all

	_, err := registry.ParseFile(context.Background(), "README.md", []byte("# hello"))
	require.Error(t, err)

	var unsupported *UnsupportedFileError
	assert.ErrorAs(t, err, &unsupported)
	assert.Equal(t, "README.md", unsupported.FilePath)
}

func TestRegistry_ParseFile_PopulatesContentSHAConsistently(t *testing.T) {
	goParser := &fakeParser{lang: LangGo, result: FileCaptures{}}
	registry := NewRegistry(goParser)

	source := []byte("package main\nfunc main() {}\n")
	fc1, err := registry.ParseFile(context.Background(), "main.go", source)
	require.NoError(t, err)

	fc2, err := registry.ParseFile(context.Background(), "main.go", source)
	require.NoError(t, err)

	// Same content -> same hash, every time. This determinism is exactly
	// what package `cache` relies on for content-addressed invalidation.
	assert.Equal(t, fc1.ContentSHA, fc2.ContentSHA)
	assert.NotEmpty(t, fc1.ContentSHA)

	changedSource := []byte("package main\nfunc main() { println(1) }\n")
	fc3, err := registry.ParseFile(context.Background(), "main.go", changedSource)
	require.NoError(t, err)
	assert.NotEqual(t, fc1.ContentSHA, fc3.ContentSHA)
}
