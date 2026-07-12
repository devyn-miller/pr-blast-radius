from typing import Protocol
from orchestrator.models.mermaid_ir import MermaidDiagramSpec
from orchestrator.models.graph_types import BlastRadiusResult, Graph

class LLMProvider(Protocol):
    name: str
    async def generate_diagram_spec(
        self, blast_radius: BlastRadiusResult, graph: Graph,
        source_snippets: dict[str, str], previous_error: str | None = None,
    ) -> MermaidDiagramSpec: ...
    async def health_check(self) -> bool: ...