# py-orchestrator/tests/test_mermaid_compiler.py
from orchestrator.models.mermaid_ir import MermaidDiagramSpec, SubgraphSpec, NodeStyle, EdgeSpec, NarrativeItem
from orchestrator.render import compile_mermaid
import pytest
from pydantic import ValidationError


def test_edges_referencing_unknown_node_raises_validation_error():
    with pytest.raises(ValidationError, match="unknown node"):
        MermaidDiagramSpec(
            subgraphs=[], nodes=[NodeStyle(id="a", label="A", risk_tier="high")],
            edges=[EdgeSpec(from_id="a", to_id="ghost")],  # "ghost" doesn't exist in nodes
            narratives=[],
        )


def test_compile_mermaid_produces_valid_flowchart_syntax():
    spec = MermaidDiagramSpec(
        subgraphs=[SubgraphSpec(id="tier_high", title="High Risk", node_ids=["a", "b"])],
        nodes=[
            NodeStyle(id="a", label="apply_discount", risk_tier="critical"),
            NodeStyle(id="b", label="OrderService.price", risk_tier="high"),
        ],
        edges=[EdgeSpec(from_id="a", to_id="b", style="solid")],
        narratives=[NarrativeItem(node_id="a", narrative="Signature changed.", risk_reasons=["public_api"])],
    )

    text = compile_mermaid(spec)

    assert text.startswith("flowchart TD")
    assert "subgraph tier_high" in text
    assert "a[" in text and "b[" in text
    assert "a --> b" in text
    assert "classDef riskCrit" in text


def test_compile_mermaid_escapes_breaking_characters_in_labels():
    node = NodeStyle(id="a", label='func["weird"] | name', risk_tier="low")
    assert '"' not in node.label
    assert "[" not in node.label
    assert "|" not in node.label