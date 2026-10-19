"""impact.md reports how many graph symbols the change does not touch.

The count comes from the Graph and Go's result; the LLM never sees or writes it.
"""
import asyncio
from pathlib import Path

from orchestrator.models.graph_types import AffectedNode, BlastRadiusResult, ChangedSymbol, Graph, Node
from orchestrator.render import count_unaffected, render_all_outputs

FIXTURES = Path(__file__).parent / "fixtures"


def _node(nid):
    return Node(ID=nid, Kind="Function", FilePath="a.py", Name=nid, QualifiedName=nid,
                Language="python", Exported=True, StartLine=1, EndLine=2)


def _affected(nid):
    return AffectedNode(NodeID=nid, Distance=1, Path=[nid], SeverityScore=0.5)


def _br(roots, affected):
    return BlastRadiusResult(
        RootSymbols=[ChangedSymbol(SymbolID=r, ChangeType="Modified") for r in roots],
        AffectedNodes=[_affected(a) for a in affected],
        MaxDepthUsed=3, TotalNodesAffected=len(affected), TotalEdgesTraversed=0)


def test_count_is_total_minus_roots_minus_affected():
    graph = Graph(Nodes={n: _node(n) for n in "abcdef"})
    assert count_unaffected(graph, _br(["a"], ["b", "c"])) == 3


def test_root_that_is_also_affected_is_not_subtracted_twice():
    graph = Graph(Nodes={n: _node(n) for n in "abcd"})
    assert count_unaffected(graph, _br(["a"], ["a", "b"])) == 2


def test_ids_missing_from_the_graph_are_ignored():
    graph = Graph(Nodes={n: _node(n) for n in "abc"})
    assert count_unaffected(graph, _br(["gone"], ["a"])) == 2


def test_tinyshop_count_matches_fixture():
    graph = Graph.model_validate_json((FIXTURES / "tinyshop_graph.json").read_text())
    br = BlastRadiusResult.model_validate_json((FIXTURES / "tinyshop_blast_radius.json").read_text())
    touched = {r.SymbolID for r in br.RootSymbols} | {a.NodeID for a in br.AffectedNodes}
    assert count_unaffected(graph, br) == len(graph.Nodes) - len(touched)
    assert count_unaffected(graph, br) > 0  # the inventory route stays unaffected


def test_impact_md_has_line_under_the_diagram(tmp_path):
    graph = Graph(Nodes={n: _node(n) for n in "abcde"})
    paths = asyncio.run(render_all_outputs("graph TD\n  a --> b", _br(["a"], ["b"]),
                                           out_dir=str(tmp_path), graph=graph))
    text = Path(paths.markdown).read_text()
    assert "3 unaffected symbols" in text
    assert text.index("```\n", text.index("```mermaid") + 3) < text.index("3 unaffected symbols")


def test_singular_wording(tmp_path):
    graph = Graph(Nodes={n: _node(n) for n in "abc"})
    paths = asyncio.run(render_all_outputs("graph TD\n  a --> b", _br(["a"], ["b"]),
                                           out_dir=str(tmp_path), graph=graph))
    assert "1 unaffected symbol\n" in Path(paths.markdown).read_text()
