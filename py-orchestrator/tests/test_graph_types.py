import json
from pathlib import Path

from orchestrator.models.graph_types import AffectedNode, Graph, Node

FIXTURES = Path(__file__).parent / "fixtures"


def test_graph_defaults_are_not_shared_between_instances():
    first = Graph(Nodes={"one": Node(
        ID="one",
        Kind="Function",
        FilePath="/tmp/one.py",
        Name="one",
        QualifiedName="one",
        Language="python",
        Exported=False,
        StartLine=1,
        EndLine=2,
    )})
    second = Graph(Nodes={})

    first.OutEdges["one"] = []
    first.Metadata.RepoPath = "/repo/a"

    assert second.OutEdges == {}
    assert second.InEdges == {}
    assert second.Metadata.RepoPath == ""


def test_affected_node_default_lists_are_not_shared():
    first = AffectedNode(NodeID="one", Distance=1, Path=["root", "one"], SeverityScore=0.4)
    second = AffectedNode(NodeID="two", Distance=2, Path=["root", "two"], SeverityScore=0.6)

    first.SeverityReasons.append("changed signature")

    assert second.SeverityReasons == []

def test_graph_metadata_parses_real_go_payload():
    # tinyshop_graph.json is verbatim get_dependency_graph output from the
    # Go binary (RepoPath made relative). Go marshals struct field names
    # as-is, so a snake_case model would silently drop every value here.
    raw = json.loads((FIXTURES / "tinyshop_graph.json").read_text())
    graph = Graph.model_validate(raw)

    assert raw["Metadata"]["UnresolvedCallCount"] > 0
    assert graph.Metadata.UnresolvedCallCount == raw["Metadata"]["UnresolvedCallCount"]
    assert graph.Metadata.RepoPath == "examples/tinyshop"
    assert set(type(graph.Metadata).model_fields) == set(raw["Metadata"])
