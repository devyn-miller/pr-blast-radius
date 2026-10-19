"""Deterministic MermaidDiagramSpec construction.

Everything structural in the diagram is derived here from Go's output: which
nodes appear, their labels, risk tiers, subgraph grouping, edges and edge
styles. LLM providers only write narratives for nodes this module has already
placed (see narrative_targets / parse_narratives).
"""
import re

from orchestrator.models.graph_types import BlastRadiusResult, Graph, Node
from orchestrator.models.mermaid_ir import (
    EdgeSpec, MermaidDiagramSpec, NarrativeItem, NarrativeSpec, NodeStyle, SubgraphSpec,
)

# Risk tier cutoffs, and the only place they are defined.
#
# They are calibrated against the REACHABLE range of Go's severity score, not
# against [0, 1]. An affected node is always at distance >= 1, which caps the
# distance term at half its weight, so with the default weights the ceiling is
# 0.825 for a route handler and 0.625 for any other node (see
# severity.MaxReachableScore in Go, and test_risk_tiers.py, which recomputes
# these from the weights and fails if a cutoff no longer fits under them).
# Cutoffs picked for [0, 1] instead leave the top tier unreachable.
CRITICAL_CUTOFF = 0.5
HIGH_CUTOFF = 0.35
MEDIUM_CUTOFF = 0.2

# Severity score -> risk tier, checked highest first. A score below every
# cutoff is "low". Changed root symbols have no severity score and use
# CHANGED_TIER instead.
RISK_THRESHOLDS: list[tuple[float, str]] = [
    (CRITICAL_CUTOFF, "critical"),
    (HIGH_CUTOFF, "high"),
    (MEDIUM_CUTOFF, "medium"),
]
LOWEST_TIER = "low"
CHANGED_TIER = "changed"

# Edges whose Go resolution confidence is below this are drawn dashed.
DASHED_BELOW_CONFIDENCE = 0.6

# Tiers whose nodes get a narrative, in addition to every changed root.
NARRATIVE_TIERS = frozenset({"critical", "high"})

# Subgraph order and titles, most severe first after the changed roots.
SUBGRAPH_ORDER: list[tuple[str, str]] = [
    (CHANGED_TIER, "Changed"),
    ("critical", "Critical Risk"),
    ("high", "High Risk"),
    ("medium", "Medium Risk"),
    ("low", "Low Risk"),
]

MAX_LABEL_LEN = 60

# Go astparse.KindModule: the synthetic node for a file's module-level code.
MODULE_KIND = "Module"


def risk_tier(severity_score: float) -> str:
    """Pure function of Go's severity score; nothing else influences tiers."""
    for threshold, tier in RISK_THRESHOLDS:
        if severity_score >= threshold:
            return tier
    return LOWEST_TIER


def mermaid_id(node_id: str) -> str:
    # Full Go ID (no truncation, so no collisions), prefixed so a Mermaid
    # identifier never starts with a digit.
    return "n_" + re.sub(r"[^A-Za-z0-9_]", "_", node_id)


def node_label(node: Node, change_type: str | None = None) -> str:
    """QualifiedName (Name as fallback), never the node ID.

    Go's synthetic per-file Module node (calls made outside any function or
    class) has no meaningful name, so it is labelled by its file.
    """
    if node.Kind == MODULE_KIND:
        name = f"{node.FilePath} (module level)"
    else:
        name = node.QualifiedName or node.Name or f"{node.FilePath}:{node.StartLine}"
    if change_type is None:
        return name[:MAX_LABEL_LEN]
    suffix = f" ({change_type})"
    return name[: MAX_LABEL_LEN - len(suffix)] + suffix


def edge_confidence(graph: Graph, callee_id: str, caller_id: str) -> float:
    """Strongest Go confidence among the caller's call edges to the callee.

    The diagram draws changed -> impacted, i.e. callee -> caller, but Go
    stores the call as caller -> callee. With several call sites, one
    certain call makes the dependency certain, so take the maximum.
    Returns 0.0 if Go has no such edge.
    """
    return max(
        (e.Confidence for e in graph.OutEdges.get(caller_id, []) if e.To == callee_id),
        default=0.0,
    )


def build_spec(blast_radius: BlastRadiusResult, graph: Graph) -> MermaidDiagramSpec:
    """Build the full diagram spec from Go's results, with no narratives."""
    nodes: dict[str, NodeStyle] = {}

    for root in blast_radius.RootSymbols:
        node = graph.Nodes.get(root.SymbolID)
        if node is None:
            continue
        mid = mermaid_id(root.SymbolID)
        nodes[mid] = NodeStyle(id=mid, label=node_label(node, root.ChangeType), risk_tier=CHANGED_TIER)

    for an in blast_radius.AffectedNodes:
        node = graph.Nodes.get(an.NodeID)
        mid = mermaid_id(an.NodeID)
        if node is None or mid in nodes:
            continue
        nodes[mid] = NodeStyle(id=mid, label=node_label(node), risk_tier=risk_tier(an.SeverityScore))

    subgraphs = []
    for tier, title in SUBGRAPH_ORDER:
        ids = [n.id for n in nodes.values() if n.risk_tier == tier]
        if ids:
            subgraphs.append(SubgraphSpec(id=f"tier_{tier}", title=title, node_ids=ids))

    edges = []
    for an in blast_radius.AffectedNodes:
        if len(an.Path) < 2:
            continue
        callee, caller = an.Path[-2], an.NodeID
        from_id, to_id = mermaid_id(callee), mermaid_id(caller)
        if from_id not in nodes or to_id not in nodes:
            continue
        dashed = edge_confidence(graph, callee, caller) < DASHED_BELOW_CONFIDENCE
        edges.append(EdgeSpec(from_id=from_id, to_id=to_id, style="dashed" if dashed else "solid"))

    return MermaidDiagramSpec(subgraphs=subgraphs, nodes=list(nodes.values()), edges=edges, narratives=[])


def narrative_targets(spec: MermaidDiagramSpec) -> list[str]:
    """Node IDs that get a narrative: every changed root plus critical/high nodes."""
    return [n.id for n in spec.nodes if n.risk_tier == CHANGED_TIER or n.risk_tier in NARRATIVE_TIERS]


def allowed_reasons(blast_radius: BlastRadiusResult) -> dict[str, list[str]]:
    """Go-computed risk reasons per Mermaid node ID; a root's reason is its change type."""
    reasons = {mermaid_id(an.NodeID): list(an.SeverityReasons) for an in blast_radius.AffectedNodes}
    for root in blast_radius.RootSymbols:
        reasons[mermaid_id(root.SymbolID)] = [root.ChangeType]
    return reasons


def parse_narratives(raw_json: str, spec: MermaidDiagramSpec,
                     blast_radius: BlastRadiusResult) -> list[NarrativeItem]:
    """Validate an LLM's narratives against the built spec.

    The LLM supplies prose and a node ID, and nothing else: raises
    pydantic.ValidationError (which the pipeline retries) if a narrative names
    a node that is not in the spec. risk_reasons is NOT read from the output —
    it is filled here from Go's SeverityReasons, so a provider cannot add,
    drop or reword a reason even if it emits the field.
    """
    context = {"node_ids": {n.id for n in spec.nodes}}
    parsed = NarrativeSpec.model_validate_json(raw_json, context=context).narratives
    reasons = allowed_reasons(blast_radius)
    return [
        NarrativeItem(node_id=item.node_id, narrative=item.narrative,
                      risk_reasons=reasons.get(item.node_id, []))
        for item in parsed
    ]
