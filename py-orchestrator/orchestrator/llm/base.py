from typing import Protocol
from orchestrator.models.mermaid_ir import MermaidDiagramSpec, NarrativeItem
from orchestrator.models.graph_types import BlastRadiusResult, Graph

class LLMProvider(Protocol):
    """Writes narratives for a spec that spec_builder.build_spec already built.

    Providers never decide nodes, labels, tiers, grouping or edges, and never
    supply risk_reasons: those come from Go's SeverityReasons, filled in by
    spec_builder. LLM-backed providers return the result of
    spec_builder.parse_narratives, so an unknown node ID raises
    ValidationError and the pipeline retries.
    """
    name: str
    async def generate_narratives(
        self, spec: MermaidDiagramSpec, blast_radius: BlastRadiusResult, graph: Graph,
        source_snippets: dict[str, str], previous_error: str | None = None,
    ) -> list[NarrativeItem]: ...
    async def health_check(self) -> bool: ...
