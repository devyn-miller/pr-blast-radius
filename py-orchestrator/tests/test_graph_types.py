from orchestrator.models.graph_types import AffectedNode, Graph, Node


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
    first.Metadata.repo_path = "/repo/a"

    assert second.OutEdges == {}
    assert second.InEdges == {}
    assert second.Metadata.repo_path == ""


def test_affected_node_default_lists_are_not_shared():
    first = AffectedNode(NodeID="one", Distance=1, Path=["root", "one"], SeverityScore=0.4)
    second = AffectedNode(NodeID="two", Distance=2, Path=["root", "two"], SeverityScore=0.6)

    first.SeverityReasons.append("changed signature")

    assert second.SeverityReasons == []