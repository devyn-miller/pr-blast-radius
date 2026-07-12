from orchestrator.models.mermaid_ir import (
    MermaidDiagramSpec, SubgraphSpec, NodeStyle, EdgeSpec, NarrativeItem
)
from orchestrator.models.graph_types import BlastRadiusResult, Graph

RISK_THRESHOLDS = [(0.7, "critical"), (0.5, "high"), (0.3, "medium")]

def _risk_tier(score: float) -> str:
    for threshold, tier in RISK_THRESHOLDS:
        if score >= threshold:
            return tier
    return "low"

class TemplateOnlyProvider:
    name = "template-only"

    async def health_check(self) -> bool:
        return True

    async def generate_diagram_spec(self, blast_radius: BlastRadiusResult, graph: Graph,
                                     source_snippets: dict[str, str], previous_error=None) -> MermaidDiagramSpec:
        safe = lambda s: "".join(c if c.isalnum() else "_" for c in s)[:20]

        nodes, narratives = [], []
        for an in blast_radius.AffectedNodes:
            node = graph.Nodes.get(an.NodeID)
            if not node:
                continue
            tier = _risk_tier(an.SeverityScore)
            nodes.append(NodeStyle(id=safe(an.NodeID), label=node.QualifiedName[:60], risk_tier=tier))
            if tier in ("critical", "high"):
                reasons = ", ".join(an.SeverityReasons) or "elevated severity score"
                narratives.append(NarrativeItem(
                    node_id=safe(an.NodeID),
                    narrative=f"Reached at distance {an.Distance} from the change. Flagged: {reasons}."[:280],
                    risk_reasons=an.SeverityReasons,
                ))

        by_tier: dict[str, list[str]] = {}
        for n in nodes:
            by_tier.setdefault(n.risk_tier, []).append(n.id)
        subgraphs = [SubgraphSpec(id=f"tier_{t}", title=f"{t.capitalize()} Risk", node_ids=ids)
                     for t, ids in by_tier.items()]

        node_ids = {n.id for n in nodes}
        edges = []
        for an in blast_radius.AffectedNodes:
            fid = safe(an.NodeID)
            if fid not in node_ids or len(an.Path) < 2:
                continue
            parent_id = safe(an.Path[-2])
            if parent_id in node_ids:
                edges.append(EdgeSpec(from_id=parent_id, to_id=fid, style="solid"))

        return MermaidDiagramSpec(subgraphs=subgraphs, nodes=nodes, edges=edges, narratives=narratives)