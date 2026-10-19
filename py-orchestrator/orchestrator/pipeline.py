import time, logging
from dataclasses import dataclass, field

from orchestrator.mcp_client import BlastRadiusMCPClient
from orchestrator.llm.resolve import resolve_provider
from orchestrator.llm.template_provider import TemplateOnlyProvider
from orchestrator.models.graph_types import ChangedSymbol, Graph, BlastRadiusResult
from orchestrator.models.mermaid_ir import MermaidDiagramSpec
from orchestrator.render import compile_mermaid, render_all_outputs, OutputPaths
from orchestrator.errors import NoChangesDetectedError, ProviderTransientError
from orchestrator.spec_builder import build_spec
from pydantic import ValidationError

logger = logging.getLogger("blastradius")
MAX_RETRIES = 2


@dataclass
class GenerationOutcome:
    provider: str
    attempts: int
    degraded: bool


@dataclass
class PipelineContext:
    repo_path: str
    ref_range: str
    max_depth: int
    top_n_for_source: int = 5

    changed_symbols: list[ChangedSymbol] | None = None
    graph: Graph | None = None
    blast_radius: BlastRadiusResult | None = None
    source_snippets: dict[str, str] = field(default_factory=dict)
    llm_provider: object | None = None
    diagram_spec: MermaidDiagramSpec | None = None
    generation_outcome: GenerationOutcome | None = None
    mermaid_text: str | None = None
    output_paths: OutputPaths | None = None


def _one_line(err: Exception) -> str:
    """Pydantic errors span several lines; keep each log record on one."""
    return " | ".join(line.strip() for line in str(err).splitlines() if line.strip())


async def generate_validated_spec(provider, blast_radius, graph, source_snippets):
    """Build the diagram deterministically, then let the provider add narratives.

    Structure (nodes, labels, tiers, subgraphs, edges) always comes from
    build_spec; the provider can only contribute narratives, and an invalid
    narrative set (unknown node, invented reason) is retried, then replaced by
    template narratives.
    """
    spec = build_spec(blast_radius, graph)
    last_error = None
    for attempt in range(MAX_RETRIES + 1):
        try:
            narratives = await provider.generate_narratives(
                spec, blast_radius, graph, source_snippets,
                previous_error=str(last_error) if last_error else None,
            )
            return (spec.model_copy(update={"narratives": narratives}),
                    GenerationOutcome(provider.name, attempt + 1, degraded=False))
        except (ValidationError, ValueError) as e:
            last_error, last_kind = e, "validation error"
            logger.warning(f"[{provider.name}] attempt {attempt+1}/{MAX_RETRIES+1} "
                           f"validation failed: {_one_line(e)}")
        except ProviderTransientError as e:
            last_error, last_kind = e, "provider error"
            logger.warning(f"[{provider.name}] attempt {attempt+1}/{MAX_RETRIES+1} "
                           f"transient error: {_one_line(e)}")

    # Self-contained so the reason is visible even if the per-attempt lines
    # scroll away (the CLI logs WARNING and above without --verbose).
    logger.warning(f"[{provider.name}] degrading to template-only after {MAX_RETRIES+1} failed "
                   f"attempts; last {last_kind} (attempt {MAX_RETRIES+1}): {_one_line(last_error)}")
    fallback = TemplateOnlyProvider()
    narratives = await fallback.generate_narratives(spec, blast_radius, graph, source_snippets)
    return (spec.model_copy(update={"narratives": narratives}),
            GenerationOutcome(fallback.name, MAX_RETRIES + 1, degraded=True))


class Pipeline:
    def __init__(self, mcp_client: BlastRadiusMCPClient, llm_preference: str = "auto"):
        self._client = mcp_client
        self._llm_preference = llm_preference

    async def run(self, ctx: PipelineContext) -> PipelineContext:
        stages = [
            self._extract_changes, self._build_graph, self._compute_blast_radius,
            self._fetch_top_n_source, self._resolve_llm_provider,
            self._generate_diagram_spec, self._compile_mermaid, self._render_outputs,
        ]
        for fn in stages:
            t0 = time.monotonic()
            ctx = await fn(ctx)
            logger.debug(f"[stage] {fn.__name__} in {time.monotonic()-t0:.2f}s")
        return ctx

    async def _extract_changes(self, ctx):
        ctx.changed_symbols = await self._client.get_changed_symbols(ctx.ref_range)
        if not ctx.changed_symbols:
            raise NoChangesDetectedError(f"No symbol changes found for ref_range={ctx.ref_range!r}")
        return ctx

    async def _build_graph(self, ctx):
        ctx.graph = await self._client.get_dependency_graph()
        return ctx

    async def _compute_blast_radius(self, ctx):
        ctx.blast_radius = await self._client.compute_blast_radius(ctx.ref_range, ctx.max_depth)
        return ctx

    async def _fetch_top_n_source(self, ctx):
        # Nodes with no source range (Go's synthetic Module node for
        # module-level code, StartLine 0) have nothing to fetch, and Go
        # rejects them; skip them rather than abort the run.
        def has_source(an):
            node = ctx.graph.Nodes.get(an.NodeID) if ctx.graph else None
            return node is None or node.StartLine >= 1

        candidates = [an for an in ctx.blast_radius.AffectedNodes if has_source(an)]
        top_n = sorted(candidates, key=lambda n: n.SeverityScore, reverse=True)[:ctx.top_n_for_source]
        for node in top_n:
            result = await self._client.get_symbol_source(node.NodeID)
            ctx.source_snippets[node.NodeID] = result.source
        return ctx

    async def _resolve_llm_provider(self, ctx):
        ctx.llm_provider = await resolve_provider(self._llm_preference)
        return ctx

    async def _generate_diagram_spec(self, ctx):
        ctx.diagram_spec, ctx.generation_outcome = await generate_validated_spec(
            ctx.llm_provider, ctx.blast_radius, ctx.graph, ctx.source_snippets
        )
        return ctx

    async def _compile_mermaid(self, ctx):
        ctx.mermaid_text = compile_mermaid(ctx.diagram_spec)
        return ctx

    async def _render_outputs(self, ctx):
        ctx.output_paths = await render_all_outputs(ctx.mermaid_text, ctx.blast_radius,
                                                    spec=ctx.diagram_spec, graph=ctx.graph)
        return ctx