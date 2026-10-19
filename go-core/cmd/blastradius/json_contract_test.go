package main

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// TestJSONContract_TinyShop_NoNullFields encodes the full Graph and
// BlastRadiusResult that the MCP server would send for TinyShop and fails if
// any value anywhere in the document is null. A nil slice or map marshals as
// null, which the Python client's Pydantic models either reject or have to
// special-case, so Go must always emit [] / {} for empty collections.
func TestJSONContract_TinyShop_NoNullFields(t *testing.T) {
	depGraph, blastRadius := runTinyShopPipeline(t)

	for name, v := range map[string]any{"Graph": depGraph, "BlastRadiusResult": blastRadius} {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("unmarshal %s: %v", name, err)
		}
		if nulls := findNulls(doc, name); len(nulls) > 0 {
			t.Errorf("%s has %d null value(s):\n  %v", name, len(nulls), nulls)
		}
	}
}

// findNulls returns the JSON path of every null in doc. Map keys that are
// node IDs are collapsed to "*" so one nil field reports once per field
// shape rather than once per node.
func findNulls(doc any, path string) []string {
	seen := map[string]bool{}
	var walk func(v any, p string)
	walk = func(v any, p string) {
		switch x := v.(type) {
		case nil:
			seen[p] = true
		case map[string]any:
			for k, child := range x {
				key := k
				if isIDKeyedMap(p) {
					key = "*"
				}
				walk(child, p+"."+key)
			}
		case []any:
			for _, child := range x {
				walk(child, p+"[]")
			}
		}
	}
	walk(doc, path)

	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func isIDKeyedMap(path string) bool {
	return strings.HasSuffix(path, ".Nodes") || strings.HasSuffix(path, ".OutEdges") ||
		strings.HasSuffix(path, ".InEdges")
}
