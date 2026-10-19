package graph

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Python client decodes these fields as list[...]; a nil slice marshals
// as null and fails Pydantic validation, so empty results must be [].

func TestMapHunksToSymbols_NoChanges_MarshalsAsEmptyArray(t *testing.T) {
	data, err := json.Marshal(MapHunksToSymbols(newEmptyGraph(), nil))
	require.NoError(t, err)
	assert.JSONEq(t, `[]`, string(data))
}

func TestComputeBlastRadius_NoDependents_AffectedNodesMarshalsAsEmptyArray(t *testing.T) {
	g := newEmptyGraph()
	g.Nodes["leaf"] = Node{ID: "leaf"}
	result := NewTraverser(g).ComputeBlastRadius([]ChangedSymbol{{SymbolID: "leaf", ChangeType: ChangeModified}}, 3)

	data, err := json.Marshal(result.AffectedNodes)
	require.NoError(t, err)
	assert.JSONEq(t, `[]`, string(data))
}
