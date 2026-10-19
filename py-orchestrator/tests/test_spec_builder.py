"""The diagram's structure comes only from Go's output via spec_builder.

Fixtures are verbatim Go output for TinyShop with the apply_discount
signature change (WORKING_TREE, max_depth=3).
"""
import json
import re
from pathlib import Path

import pytest
from pydantic import ValidationError

from orchestrator.llm.template_provider import TemplateOnlyProvider
from orchestrator.models.graph_types import AffectedNode, BlastRadiusResult, ChangedSymbol, Graph
from orchestrator.models.mermaid_ir import NarrativeItem, NarrativeSpec
from orchestrator.pipeline import generate_validated_spec
from orchestrator.render import compile_mermaid
from orchestrator.spec_builder import (
    CHANGED_TIER, CRITICAL_CUTOFF, DASHED_BELOW_CONFIDENCE, HIGH_CUTOFF, MEDIUM_CUTOFF,
    RISK_THRESHOLDS, allowed_reasons, build_spec, mermaid_id, narrative_targets,
    parse_narratives, risk_tier,
)

FIXTURES = Path(__file__).parent / "fixtures"
HEX_HASH = re.compile(r"\b[0-9a-f]{12,}\b")


def _load_tinyshop():
    graph = Graph.model_validate_json((FIXTURES / "tinyshop_graph.json").read_text())
    br = BlastRadiusResult.model_validate_json((FIXTURES / "tinyshop_blast_radius.json").read_text())
    return graph, br


def _structure(spec):
    return (
        [n.model_dump() for n in spec.nodes],
        [e.model_dump() for e in spec.edges],
        [sg.model_dump() for sg in spec.subgraphs],
    )


class _FakeLLM:
    """Valid narratives for every node, which also 'tries' to restyle things.

    It can only return narratives, so its opinions about tiers have nowhere
    to go; the test proves the built structure survives unchanged.
    """
    name = "fake-llm"

    async def generate_narratives(self, spec, blast_radius, graph, source_snippets, previous_error=None):
        payload = {"narratives": [
            {"node_id": n.id, "narrative": f"LLM says {n.label} is CRITICAL", "risk_reasons": []}
            for n in spec.nodes
        ]}
        return parse_narratives(json.dumps(payload), spec, blast_radius)


# --- tiers ---------------------------------------------------------------

@pytest.mark.parametrize("score,tier", [
    (1.0, "critical"), (0.5, "critical"), (0.49, "high"), (0.35, "high"),
    (0.34, "medium"), (0.2, "medium"), (0.19, "low"), (0.0, "low"),
])
def test_risk_tier_is_a_pure_function_of_severity_score(score, tier):
    assert risk_tier(score) == tier
    assert risk_tier(score) == risk_tier(score)


def test_thresholds_come_from_the_named_cutoffs():
    # The cutoffs sit inside the severity score's reachable range, not [0, 1];
    # test_risk_tiers.py re-derives that range from Go's weights.
    assert RISK_THRESHOLDS == [
        (CRITICAL_CUTOFF, "critical"), (HIGH_CUTOFF, "high"), (MEDIUM_CUTOFF, "medium"),
    ]
    assert (CRITICAL_CUTOFF, HIGH_CUTOFF, MEDIUM_CUTOFF) == (0.5, 0.35, 0.2)


async def test_fake_llm_cannot_change_tiers_or_structure():
    graph, br = _load_tinyshop()
    built = build_spec(br, graph)

    spec, outcome = await generate_validated_spec(_FakeLLM(), br, graph, {})

    assert outcome.provider == "fake-llm" and outcome.degraded is False
    assert _structure(spec) == _structure(built)
    by_go_id = {an.NodeID: an for an in br.AffectedNodes}
    for n in spec.nodes:
        if n.risk_tier != CHANGED_TIER:
            go_id = next(g for g in by_go_id if mermaid_id(g) == n.id)
            assert n.risk_tier == risk_tier(by_go_id[go_id].SeverityScore)


def test_risk_reasons_are_not_part_of_the_llm_schema():
    """The schema sent to Gemini/Ollama has no risk_reasons field at all."""
    schema = NarrativeSpec.model_json_schema()
    item = schema["$defs"]["LLMNarrative"]["properties"]

    assert set(item) == {"node_id", "narrative"}
    assert "risk_reasons" not in json.dumps(schema)


@pytest.mark.parametrize("sent", [
    ["critical_path"],            # invented outright
    [],                           # dropped
    ["public_api", "made_up"],    # real reason plus an invented one
    "not even a list",
])
def test_fake_llm_output_cannot_change_risk_reasons(sent):
    """Whatever a provider puts in risk_reasons, Go's reasons are what survive."""
    graph, br = _load_tinyshop()
    spec = build_spec(br, graph)
    go_id = "0208706ac4c993a1869cc6231a7e9ed30f4caa82"  # OrderService.price
    price = mermaid_id(go_id)
    from_go = next(an.SeverityReasons for an in br.AffectedNodes if an.NodeID == go_id)
    assert from_go, "fixture must give price at least one reason for this to prove anything"

    payload = {"narratives": [{"node_id": price, "narrative": "x", "risk_reasons": sent}]}
    items = parse_narratives(json.dumps(payload), spec, br)

    assert [i.risk_reasons for i in items] == [from_go]


def test_template_and_llm_risk_reasons_agree_for_the_same_node():
    graph, br = _load_tinyshop()
    spec = build_spec(br, graph)
    expected = allowed_reasons(br)

    for nid in narrative_targets(spec):
        payload = {"narratives": [{"node_id": nid, "narrative": "x"}]}
        item, = parse_narratives(json.dumps(payload), spec, br)
        assert item.risk_reasons == expected[nid]


def test_unknown_narrative_node_fails_validation():
    graph, br = _load_tinyshop()
    spec = build_spec(br, graph)
    payload = {"narratives": [{"node_id": "n_ghost", "narrative": "x", "risk_reasons": []}]}

    with pytest.raises(ValidationError, match="unknown node: n_ghost"):
        parse_narratives(json.dumps(payload), spec, br)


# --- labels --------------------------------------------------------------

def test_no_label_is_a_node_id_or_hex_hash():
    graph, br = _load_tinyshop()
    spec = build_spec(br, graph)
    go_ids = set(graph.Nodes)

    for n in spec.nodes:
        assert n.label not in go_ids
        assert n.label != n.id
        assert not HEX_HASH.search(n.label), f"label looks like a hash: {n.label}"


def test_labels_are_qualified_names_with_change_type_on_roots():
    graph, br = _load_tinyshop()
    labels = {n.label for n in build_spec(br, graph).nodes}

    assert labels == {
        "DiscountEngine (Modified)",
        "DiscountEngine.apply_discount (SignatureChanged)",
        "OrderService.price",
        "create_order",
        "quote_order",
        "price_cart",
        "checkout",
        "api/routes.py (module level)",  # Go's synthetic Module node, labelled by file
    }


def test_label_falls_back_to_name_never_id():
    graph, _ = _load_tinyshop()
    go_id = "0208706ac4c993a1869cc6231a7e9ed30f4caa82"
    graph.Nodes[go_id] = graph.Nodes[go_id].model_copy(update={"QualifiedName": ""})
    br = BlastRadiusResult(
        RootSymbols=[], MaxDepthUsed=3, TotalNodesAffected=1, TotalEdgesTraversed=0,
        AffectedNodes=[AffectedNode(NodeID=go_id, Distance=1, Path=[go_id], SeverityScore=0.1)],
    )

    assert [n.label for n in build_spec(br, graph).nodes] == ["price"]


# --- structure -----------------------------------------------------------

def test_tinyshop_structure_tiers_and_edges():
    graph, br = _load_tinyshop()
    spec = build_spec(br, graph)
    by_label = {n.label: n for n in spec.nodes}

    engine = by_label["DiscountEngine (Modified)"]
    apply_discount = by_label["DiscountEngine.apply_discount (SignatureChanged)"]
    price = by_label["OrderService.price"]
    handlers = [by_label[h] for h in ("create_order", "quote_order", "price_cart", "checkout")]
    module = by_label["api/routes.py (module level)"]

    assert engine.risk_tier == apply_discount.risk_tier == CHANGED_TIER
    # Go's severity scores, against cutoffs critical 0.5 / high 0.35 / medium
    # 0.2: price 0.615 (distance 1 AND the graph's fan-in maximum, in-degree 4
    # from the four handlers), each handler 0.3967 (distance 2, no fan-in of
    # its own, route handler), module 0.275.
    assert price.risk_tier == "critical"
    assert [h.risk_tier for h in handlers] == ["high"] * 4
    assert module.risk_tier == "medium"
    assert [sg.id for sg in spec.subgraphs] == [
        "tier_changed", "tier_critical", "tier_high", "tier_medium",
    ]

    edges = {(e.from_id, e.to_id): e.style for e in spec.edges}
    assert edges == {
        # self.pricing typed from `pricing: DiscountEngine` -> 0.9, solid
        (apply_discount.id, price.id): "solid",
        # orders = OrderService(...) constructor-tracked -> 0.8, solid
        **{(price.id, h.id): "solid" for h in handlers},
        # module-level DiscountEngine() direct call -> 1.0, solid
        (engine.id, module.id): "solid",
    }
    assert spec.narratives == []


def test_low_confidence_edge_is_dashed():
    graph, br = _load_tinyshop()
    callee = "15a3ec7ef1c21e519e0d30d36c58a920b3b1110c"  # apply_discount
    caller = "0208706ac4c993a1869cc6231a7e9ed30f4caa82"  # OrderService.price
    for e in graph.OutEdges[caller]:
        if e.To == callee:
            e.Confidence = 0.3  # what Go emitted before self.X hints (duck-typed)
    edge = next(e for e in build_spec(br, graph).edges
                if (e.from_id, e.to_id) == (mermaid_id(callee), mermaid_id(caller)))

    assert 0.3 < DASHED_BELOW_CONFIDENCE
    assert edge.style == "dashed"


async def test_template_and_fake_llm_paths_differ_only_in_narratives():
    graph, br = _load_tinyshop()

    template_spec, template_outcome = await generate_validated_spec(TemplateOnlyProvider(), br, graph, {})
    llm_spec, llm_outcome = await generate_validated_spec(_FakeLLM(), br, graph, {})

    assert template_outcome.provider == "template-only" and llm_outcome.provider == "fake-llm"
    assert _structure(template_spec) == _structure(llm_spec)
    assert compile_mermaid(template_spec) == compile_mermaid(llm_spec)
    assert template_spec.narratives != llm_spec.narratives


async def test_template_narratives_cover_roots_and_high_tiers_only():
    graph, br = _load_tinyshop()
    spec = build_spec(br, graph)

    narratives = await TemplateOnlyProvider().generate_narratives(spec, br, graph, {})

    tier = {n.id: n.risk_tier for n in spec.nodes}
    # The two changed roots, plus price (critical) and the four route handlers
    # (high). The module node is "medium" and gets no narrative.
    assert {tier[i.node_id] for i in narratives} == {CHANGED_TIER, "critical", "high"}
    assert len(narratives) == 7
    # Template narratives pass the same validation an LLM's must.
    payload = json.dumps({"narratives": [i.model_dump() for i in narratives]})
    assert parse_narratives(payload, spec, br) == narratives
    assert all(isinstance(i, NarrativeItem) for i in narratives)
