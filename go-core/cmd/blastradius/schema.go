package main

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/graph"
)

// modulePath scopes the schema walk to this module's own types; stdlib types
// such as time.Time are leaves, not structs to describe.
const modulePath = "github.com/devyn-miller/pr-blast-radius/go-core/"

// contractRoots are the values the MCP server JSON-encodes for the Python
// client. Every project struct reachable from them is included.
var contractRoots = []any{
	graph.Graph{},
	graph.BlastRadiusResult{},
	graph.ChangedSymbol{},
}

// contractSchema maps each reachable struct type name to its JSON field
// names, in declaration order. It is the source of truth that
// py-orchestrator/scripts/check_contract.py diffs the Pydantic models against.
func contractSchema() map[string][]string {
	schema := map[string][]string{}
	var visit func(t reflect.Type)
	visit = func(t reflect.Type) {
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			visit(t.Elem())
			return
		case reflect.Map:
			visit(t.Key())
			visit(t.Elem())
			return
		case reflect.Struct:
		default:
			return
		}
		if !strings.HasPrefix(t.PkgPath(), modulePath) {
			return
		}
		if _, done := schema[t.Name()]; done {
			return
		}
		fields := []string{}
		schema[t.Name()] = fields
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := f.Name
			if tag, ok := f.Tag.Lookup("json"); ok {
				tagName, _, _ := strings.Cut(tag, ",")
				if tagName == "-" {
					continue
				}
				if tagName != "" {
					name = tagName
				}
			}
			fields = append(fields, name)
			visit(f.Type)
		}
		schema[t.Name()] = fields
	}
	for _, root := range contractRoots {
		visit(reflect.TypeOf(root))
	}
	return schema
}

func printSchema(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(contractSchema())
}
