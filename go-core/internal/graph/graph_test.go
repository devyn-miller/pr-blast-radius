package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
)

// capturesParser returns pre-built captures per path, standing in for the
// tree-sitter registry.
type capturesParser map[string]astparse.FileCaptures

func (p capturesParser) ParseFile(_ context.Context, path string, _ []byte) (astparse.FileCaptures, error) {
	return p[path], nil
}

func TestBuildGraph_ModuleLevelCall_RegistersFileLevelNode(t *testing.T) {
	parser := capturesParser{
		"engine.py": {
			FilePath: "engine.py", Language: astparse.LangPython,
			Symbols: []astparse.SymbolCapture{
				{Name: "DiscountEngine", Kind: astparse.KindClass, StartLine: 1, EndLine: 5},
			},
		},
		"routes.py": {
			FilePath: "routes.py", Language: astparse.LangPython,
			// orders = OrderService(DiscountEngine()) at module level: no
			// enclosing symbol, so the call belongs to the file-level node.
			Calls: []astparse.CallCapture{{Kind: astparse.CallDirect, DirectName: "DiscountEngine", SiteLine: 3}},
		},
	}
	g, err := BuildGraph(context.Background(), parser, []FileSource{{Path: "engine.py"}, {Path: "routes.py"}})
	require.NoError(t, err)

	moduleID := computeNodeID("routes.py", fileLevelQualifiedName)
	node, ok := g.Nodes[moduleID]
	require.True(t, ok, "every edge endpoint must be a registered node")
	assert.Equal(t, astparse.KindModule, node.Kind)
	assert.Equal(t, "routes.py", node.FilePath)
	assert.Equal(t, 1, node.OutDegree)

	// A file with no module-level calls gets no module node.
	_, engineHasModule := g.Nodes[computeNodeID("engine.py", fileLevelQualifiedName)]
	assert.False(t, engineHasModule)

	for from, edges := range g.OutEdges {
		assert.Contains(t, g.Nodes, from)
		for _, e := range edges {
			assert.Contains(t, g.Nodes, e.To)
		}
	}
}
