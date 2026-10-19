from orchestrator.models.mermaid_ir import MermaidDiagramSpec, NarrativeItem
from orchestrator.models.graph_types import BlastRadiusResult, Graph
from orchestrator.spec_builder import (
    CHANGED_TIER, allowed_reasons, mermaid_id, narrative_targets,
)

class TemplateOnlyProvider:
    """Deterministic narratives; the always-available fallback."""
    name = "template-only"

    async def health_check(self) -> bool:
        return True

    async def generate_narratives(self, spec: MermaidDiagramSpec, blast_radius: BlastRadiusResult,
                                  graph: Graph, source_snippets: dict[str, str],
                                  previous_error=None) -> list[NarrativeItem]:
        node_by_id = {n.id: n for n in spec.nodes}
        roots = {mermaid_id(r.SymbolID): r for r in blast_radius.RootSymbols}
        affected = {mermaid_id(an.NodeID): an for an in blast_radius.AffectedNodes}
        # Same source every provider's risk_reasons come from, so a template
        # narrative and an LLM narrative for one node carry identical reasons.
        reasons_by_id = allowed_reasons(blast_radius)
        callers: dict[str, int] = {}
        for e in spec.edges:
            callers[e.from_id] = callers.get(e.from_id, 0) + 1

        narratives = []
        for nid in narrative_targets(spec):
            node = node_by_id[nid]
            reasons = reasons_by_id.get(nid, [])
            if node.risk_tier == CHANGED_TIER:
                root = roots[nid]
                n = callers.get(nid, 0)
                text = (f"Changed in this diff ({root.ChangeType}). "
                        f"{n} direct caller{'s' if n != 1 else ''} in the blast radius.")
            else:
                an = affected[nid]
                flagged = ", ".join(reasons) or "elevated severity score"
                text = f"Reached at distance {an.Distance} from the change. Flagged: {flagged}."
            narratives.append(NarrativeItem(node_id=nid, narrative=text[:280], risk_reasons=reasons))
        return narratives
