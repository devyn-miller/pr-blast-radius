"""Fail if the Pydantic models drift from the Go structs they decode.

Usage:
    blastradius --print-schema > schema.json
    python scripts/check_contract.py schema.json     # or: ... | python scripts/check_contract.py -

The Go schema maps each struct type sent to the Python client to its JSON
field names. Go marshals these structs without json tags, so the JSON keys
are the exact Go field names, and Pydantic silently ignores unknown keys:
a renamed or missing field zeroes data instead of failing. This script turns
that into a hard failure in CI.
"""
import json
import sys

from pydantic import BaseModel

from orchestrator.models import graph_types

# Go struct name -> Pydantic model. Every Go type in the schema must appear
# here, so a new Go type sent over MCP forces an explicit decision.
GO_TO_PYDANTIC: dict[str, type[BaseModel]] = {
    "Graph": graph_types.Graph,
    "Node": graph_types.Node,
    "Edge": graph_types.Edge,
    "CallSite": graph_types.CallSite,
    "Metadata": graph_types.GraphMetadata,
    "ChangedSymbol": graph_types.ChangedSymbol,
    "AffectedNode": graph_types.AffectedNode,
    "BlastRadiusResult": graph_types.BlastRadiusResult,
}


def diff_contract(go_schema: dict[str, list[str]]) -> list[str]:
    problems = []
    for go_type in sorted(go_schema):
        model = GO_TO_PYDANTIC.get(go_type)
        if model is None:
            problems.append(f"{go_type}: Go type has no Pydantic model (add it to GO_TO_PYDANTIC)")
            continue
        go_fields = set(go_schema[go_type])
        py_fields = set(model.model_fields)
        for f in sorted(go_fields - py_fields):
            problems.append(f"{go_type}.{f}: in Go, missing from {model.__name__}")
        for f in sorted(py_fields - go_fields):
            problems.append(f"{go_type}.{f}: in {model.__name__}, not sent by Go")
    for go_type in sorted(set(GO_TO_PYDANTIC) - set(go_schema)):
        problems.append(f"{go_type}: mapped to {GO_TO_PYDANTIC[go_type].__name__} but not in Go schema")
    return problems


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    src = sys.stdin if argv[1] == "-" else open(argv[1])
    with src:
        go_schema = json.load(src)

    problems = diff_contract(go_schema)
    if problems:
        print(f"Go/Python contract drift ({len(problems)}):")
        for p in problems:
            print(f"  - {p}")
        return 1
    print(f"Contract OK: {len(go_schema)} Go types match their Pydantic models.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
