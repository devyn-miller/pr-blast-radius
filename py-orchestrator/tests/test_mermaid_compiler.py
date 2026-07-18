"""Test the deterministic Mermaid diagram compiler.

The compile_mermaid function takes a validated MermaidDiagramSpec (which has
already passed Pydantic cross-field validation) and renders it as Mermaid
syntax text. This is the safety net that prevents malformed LLM output from
ever reaching the user — the LLM generates STRUCTURED DATA, then this
compiler renders it, never letting the LLM write raw Mermaid syntax.
"""
import pytest
from orchestrator.render import compile_mermaid, _escape
from orchestrator.models.mermaid_ir import (
    MermaidDiagramSpec, SubgraphSpec, NodeStyle, EdgeSpec, NarrativeItem
)


def test_compile_mermaid_basic_structure():
    """Verify the compiler produces valid Mermaid flowchart structure."""
    spec = MermaidDiagramSpec(
        direction="TD",
        subgraphs=[
            SubgraphSpec(
                id="sg1",
                title="Changed Code",
                node_ids=["n1"]
            )
        ],
        nodes=[
            NodeStyle(id="n1", label="apply_discount", risk_tier="high")
        ],
        edges=[],
        narratives=[]
    )

    result = compile_mermaid(spec)

    assert "flowchart TD" in result
    assert 'subgraph sg1["Changed Code"]' in result
    assert 'n1["apply_discount"]:::riskHigh' in result
    # Verify custom style classes are included
    assert "classDef riskHigh" in result


def test_compile_mermaid_edges_solid_and_dashed():
    """Verify edges render with correct arrow styles."""
    spec = MermaidDiagramSpec(
        direction="LR",
        subgraphs=[],
        nodes=[
            NodeStyle(id="n1", label="caller", risk_tier="medium"),
            NodeStyle(id="n2", label="callee", risk_tier="low"),
        ],
        edges=[
            EdgeSpec(from_id="n1", to_id="n2", style="solid"),
            EdgeSpec(from_id="n2", to_id="n1", style="dashed"),
        ],
        narratives=[]
    )

    result = compile_mermaid(spec)

    # Solid edge uses -->
    assert "n1 --> n2" in result
    # Dashed edge uses -.->
    assert "n2 -.-> n1" in result


def test_compile_mermaid_escapes_special_chars():
    """Verify the compiler escapes Mermaid-breaking characters in labels."""
    spec = MermaidDiagramSpec(
        direction="TD",
        subgraphs=[
            SubgraphSpec(id="sg1", title="Test", node_ids=["n1"])
        ],
        nodes=[
            NodeStyle(
                id="n1",
                label='func_with_"quotes"_and_[brackets]_and_|_pipes',
                risk_tier="critical"
            )
        ],
        edges=[],
        narratives=[]
    )

    result = compile_mermaid(spec)

    # The _escape function should have sanitized the label
    # Check that the problematic characters are removed/converted
    # Note: brackets are converted to parens, but the Pydantic validator also strips them
    assert 'n1["func_with_quotes_and_brackets_and__pipes"]:::riskCrit' in result


def test_compile_mermaid_all_risk_tiers():
    """Verify all four risk tier classes are applied correctly."""
    spec = MermaidDiagramSpec(
        direction="TD",
        subgraphs=[
            SubgraphSpec(id="sg1", title="Risk Levels", node_ids=["crit", "high", "med", "low"])
        ],
        nodes=[
            NodeStyle(id="crit", label="Critical", risk_tier="critical"),
            NodeStyle(id="high", label="High", risk_tier="high"),
            NodeStyle(id="med", label="Medium", risk_tier="medium"),
            NodeStyle(id="low", label="Low", risk_tier="low"),
        ],
        edges=[],
        narratives=[]
    )

    result = compile_mermaid(spec)

    assert 'crit["Critical"]:::riskCrit' in result
    assert 'high["High"]:::riskHigh' in result
    assert 'med["Medium"]:::riskMed' in result
    assert 'low["Low"]:::riskLow' in result


def test_compile_mermaid_multiple_subgraphs():
    """Verify multiple subgraphs are rendered in order."""
    spec = MermaidDiagramSpec(
        direction="TD",
        subgraphs=[
            SubgraphSpec(id="sg1", title="Changed", node_ids=["n1"]),
            SubgraphSpec(id="sg2", title="Affected", node_ids=["n2", "n3"]),
        ],
        nodes=[
            NodeStyle(id="n1", label="Root", risk_tier="high"),
            NodeStyle(id="n2", label="Child1", risk_tier="medium"),
            NodeStyle(id="n3", label="Child2", risk_tier="medium"),
        ],
        edges=[
            EdgeSpec(from_id="n1", to_id="n2"),
            EdgeSpec(from_id="n1", to_id="n3"),
        ],
        narratives=[]
    )

    result = compile_mermaid(spec)

    # Both subgraphs should appear
    assert 'subgraph sg1["Changed"]' in result
    assert 'subgraph sg2["Affected"]' in result
    # Nodes in correct subgraphs (check they appear after their subgraph declaration)
    sg1_pos = result.index("sg1")
    n1_pos = result.index('n1["Root"]')
    assert n1_pos > sg1_pos


def test_compile_mermaid_custom_style_block_included():
    """Verify the custom CSS class definitions are always appended."""
    spec = MermaidDiagramSpec(
        direction="TD",
        subgraphs=[],
        nodes=[NodeStyle(id="n1", label="Test", risk_tier="low")],
        edges=[],
        narratives=[]
    )

    result = compile_mermaid(spec)

    # All four risk class definitions should be present
    assert "classDef riskCrit fill:#7f1d1d" in result
    assert "classDef riskHigh fill:#c2410c" in result
    assert "classDef riskMed fill:#a16207" in result
    assert "classDef riskLow fill:#166534" in result


def test_escape_function_sanitizes_dangerous_chars():
    """Test the _escape helper directly."""
    assert _escape('has"quotes') == "hasquotes"
    assert _escape("has|pipes") == "haspipes"
    assert _escape("has[brackets]") == "has(brackets)"
    assert _escape("multi\nline") == "multi line"
    # Combined case (note: newline becomes space, parens from bracket conversion)
    assert _escape('combo"[|]\ntest') == "combo() test"


def test_compile_mermaid_empty_subgraph():
    """Verify empty subgraph (no nodes referenced) is still rendered."""
    spec = MermaidDiagramSpec(
        direction="TD",
        subgraphs=[
            SubgraphSpec(id="empty", title="Empty Group", node_ids=[])
        ],
        nodes=[],
        edges=[],
        narratives=[]
    )

    result = compile_mermaid(spec)

    # Empty subgraph should still appear in output
    assert 'subgraph empty["Empty Group"]' in result
    assert "end" in result


def test_compile_mermaid_node_not_in_any_subgraph():
    """Verify nodes can exist outside subgraphs (Pydantic allows this)."""
    spec = MermaidDiagramSpec(
        direction="TD",
        subgraphs=[],
        nodes=[
            NodeStyle(id="orphan", label="Standalone Node", risk_tier="low")
        ],
        edges=[],
        narratives=[]
    )

    result = compile_mermaid(spec)

    # Node should NOT appear in output if not in a subgraph
    # (based on the compiler logic, nodes only render inside subgraphs)
    # This is a design choice: nodes must be in subgraphs to be visible
    assert 'orphan["Standalone Node"]' not in result
