"""Test the 8-stage pipeline state machine.

The Pipeline orchestrates the full flow:
1. extract_changes (MCP: get_changed_symbols)
2. build_graph (MCP: get_dependency_graph)
3. compute_blast_radius (MCP: compute_blast_radius)
4. fetch_top_n_source (MCP: get_symbol_source for top N by severity)
5. resolve_llm_provider (LLM fallback chain: Gemini → Ollama → Template)
6. generate_diagram_spec (deterministic spec; LLM writes narratives only)
7. compile_mermaid (deterministic: spec → text)
8. render_outputs (write .md and optional .svg)

Each stage updates the PipelineContext and passes it to the next stage.
"""
import json

import pytest
from unittest.mock import AsyncMock, patch
from orchestrator.pipeline import Pipeline, PipelineContext, GenerationOutcome, MAX_RETRIES
from orchestrator.mcp_client import BlastRadiusMCPClient
from orchestrator.models.graph_types import (
    ChangedSymbol, Graph, Node, BlastRadiusResult, AffectedNode, GraphMetadata
)
from orchestrator.models.mermaid_ir import (
    MermaidDiagramSpec, SubgraphSpec, NodeStyle, EdgeSpec, NarrativeItem
)
from orchestrator.errors import NoChangesDetectedError, ProviderTransientError
from orchestrator.llm.template_provider import TemplateOnlyProvider
from orchestrator.spec_builder import build_spec, parse_narratives


@pytest.fixture
def mock_mcp_client():
    """Mock MCP client that returns synthetic data for all stages."""
    client = AsyncMock(spec=BlastRadiusMCPClient)

    # Stage 1: extract_changes returns a list of changed symbols
    client.get_changed_symbols.return_value = [
        ChangedSymbol(SymbolID="sym1", ChangeType="SignatureChanged")
    ]

    # Stage 2: build_graph returns a minimal graph
    client.get_dependency_graph.return_value = Graph(
        Nodes={
            "sym1": Node(
                ID="sym1", Kind="function", FilePath="test.py",
                Name="test_func", QualifiedName="test_func",
                Language="python", Exported=True,
                StartLine=1, EndLine=10
            )
        },
        OutEdges={},
        InEdges={},
        Metadata=GraphMetadata()
    )

    # Stage 3: compute_blast_radius returns affected nodes
    client.compute_blast_radius.return_value = BlastRadiusResult(
        RootSymbols=[ChangedSymbol(SymbolID="sym1", ChangeType="SignatureChanged")],
        AffectedNodes=[
            AffectedNode(NodeID="sym1", Distance=0, Path=["sym1"], SeverityScore=0.8, SeverityReasons=["high"])
        ],
        MaxDepthUsed=3,
        TotalNodesAffected=1,
        TotalEdgesTraversed=0
    )

    # Stage 4: fetch_top_n_source returns source code for requested symbols
    client.get_symbol_source.return_value = type('obj', (object,), {
        'symbol_id': 'sym1',
        'source': 'def test_func():\n    pass',
        'truncated': False
    })()

    return client


@pytest.mark.asyncio
async def test_pipeline_runs_all_8_stages_in_order(mock_mcp_client):
    """Verify the pipeline executes all 8 stages sequentially."""
    pipeline = Pipeline(mock_mcp_client, llm_preference="none")

    ctx = PipelineContext(
        repo_path="/fake/repo",
        ref_range="HEAD~1",
        max_depth=3
    )

    # Run the full pipeline
    result_ctx = await pipeline.run(ctx)

    # Verify each stage populated its expected field
    assert result_ctx.changed_symbols is not None
    assert result_ctx.graph is not None
    assert result_ctx.blast_radius is not None
    assert len(result_ctx.source_snippets) > 0
    assert result_ctx.llm_provider is not None
    assert result_ctx.diagram_spec is not None
    assert result_ctx.mermaid_text is not None
    assert result_ctx.output_paths is not None


@pytest.mark.asyncio
async def test_pipeline_stage1_no_changes_raises_error(mock_mcp_client):
    """Verify stage 1 raises NoChangesDetectedError when no symbols changed."""
    mock_mcp_client.get_changed_symbols.return_value = []  # Empty list

    pipeline = Pipeline(mock_mcp_client, llm_preference="none")
    ctx = PipelineContext(repo_path="/fake", ref_range="HEAD~1", max_depth=3)

    with pytest.raises(NoChangesDetectedError, match="No symbol changes found"):
        await pipeline.run(ctx)


@pytest.mark.asyncio
async def test_pipeline_stage5_llm_preference_none_uses_template(mock_mcp_client):
    """Verify llm_preference='none' always uses TemplateOnlyProvider."""
    pipeline = Pipeline(mock_mcp_client, llm_preference="none")

    ctx = PipelineContext(repo_path="/fake", ref_range="HEAD~1", max_depth=3)
    result_ctx = await pipeline.run(ctx)

    # Should have used template-only provider
    assert isinstance(result_ctx.llm_provider, TemplateOnlyProvider)
    assert result_ctx.generation_outcome.provider == "template-only"


@pytest.mark.asyncio
async def test_pipeline_stage4_fetches_top_n_sources_by_severity(mock_mcp_client):
    """Verify stage 4 fetches source for the top N highest-severity nodes."""
    # Set up blast radius with 3 nodes, different severities
    mock_mcp_client.compute_blast_radius.return_value = BlastRadiusResult(
        RootSymbols=[],
        AffectedNodes=[
            AffectedNode(NodeID="low", Distance=1, Path=["low"], SeverityScore=0.2, SeverityReasons=[]),
            AffectedNode(NodeID="high", Distance=1, Path=["high"], SeverityScore=0.9, SeverityReasons=[]),
            AffectedNode(NodeID="med", Distance=1, Path=["med"], SeverityScore=0.5, SeverityReasons=[]),
        ],
        MaxDepthUsed=3,
        TotalNodesAffected=3,
        TotalEdgesTraversed=2
    )

    pipeline = Pipeline(mock_mcp_client, llm_preference="none")
    ctx = PipelineContext(repo_path="/fake", ref_range="HEAD~1", max_depth=3, top_n_for_source=2)

    result_ctx = await pipeline.run(ctx)

    # Should have fetched source for exactly top_n_for_source=2 nodes
    # The two highest severity: "high" (0.9) and "med" (0.5)
    assert len(result_ctx.source_snippets) == 2
    assert "high" in result_ctx.source_snippets
    assert "med" in result_ctx.source_snippets
    assert "low" not in result_ctx.source_snippets


@pytest.mark.asyncio
async def test_pipeline_stage7_compiles_mermaid_from_spec(mock_mcp_client):
    """Verify stage 7 produces deterministic Mermaid text from the spec."""
    pipeline = Pipeline(mock_mcp_client, llm_preference="none")
    ctx = PipelineContext(repo_path="/fake", ref_range="HEAD~1", max_depth=3)

    result_ctx = await pipeline.run(ctx)

    # The template provider produces a valid MermaidDiagramSpec
    assert result_ctx.mermaid_text is not None
    assert "flowchart" in result_ctx.mermaid_text
    assert "classDef" in result_ctx.mermaid_text  # Custom styles included


@pytest.mark.asyncio
async def test_pipeline_stage8_writes_markdown_output(mock_mcp_client, tmp_path):
    """Verify stage 8 writes impact.md with embedded Mermaid diagram."""
    pipeline = Pipeline(mock_mcp_client, llm_preference="none")
    ctx = PipelineContext(repo_path="/fake", ref_range="HEAD~1", max_depth=3)

    # Patch render_all_outputs to use tmp_path
    with patch("orchestrator.pipeline.render_all_outputs") as mock_render:
        from orchestrator.render import OutputPaths
        mock_render.return_value = OutputPaths(
            markdown=str(tmp_path / "impact.md"),
            svg=None  # SVG generation may fail if mmdc not installed
        )

        result_ctx = await pipeline.run(ctx)

    assert result_ctx.output_paths is not None
    assert result_ctx.output_paths.markdown.endswith("impact.md")


@pytest.mark.asyncio
async def test_pipeline_stage4_skips_nodes_without_source_range(mock_mcp_client):
    """Go's module-level node has StartLine 0; fetching its source would fail the run."""
    mock_mcp_client.get_dependency_graph.return_value = Graph(Nodes={
        "mod": Node(ID="mod", Kind="Module", FilePath="api/routes.py", Name="<module>",
                    QualifiedName="<module>", Language="python", Exported=False, StartLine=0, EndLine=0),
        "fn": Node(ID="fn", Kind="Function", FilePath="api/routes.py", Name="create_order",
                   QualifiedName="create_order", Language="python", Exported=True, StartLine=5, EndLine=9),
    })
    mock_mcp_client.compute_blast_radius.return_value = BlastRadiusResult(
        RootSymbols=[], MaxDepthUsed=3, TotalNodesAffected=2, TotalEdgesTraversed=2,
        AffectedNodes=[
            AffectedNode(NodeID="mod", Distance=1, Path=["mod"], SeverityScore=0.9),
            AffectedNode(NodeID="fn", Distance=1, Path=["fn"], SeverityScore=0.4),
        ],
    )

    pipeline = Pipeline(mock_mcp_client, llm_preference="none")
    result_ctx = await pipeline.run(PipelineContext(repo_path="/fake", ref_range="HEAD~1", max_depth=3))

    assert list(result_ctx.source_snippets) == ["fn"]
    requested = [c.args[0] for c in mock_mcp_client.get_symbol_source.call_args_list]
    assert requested == ["fn"]


@pytest.mark.asyncio
async def test_pipeline_context_carries_state_through_stages(mock_mcp_client):
    """Verify each stage reads from and writes to the shared PipelineContext."""
    pipeline = Pipeline(mock_mcp_client, llm_preference="none")

    initial_ctx = PipelineContext(
        repo_path="/test/repo",
        ref_range="main...feature",
        max_depth=5,
        top_n_for_source=3
    )

    final_ctx = await pipeline.run(initial_ctx)

    # Initial values should be preserved
    assert final_ctx.repo_path == "/test/repo"
    assert final_ctx.ref_range == "main...feature"
    assert final_ctx.max_depth == 5
    assert final_ctx.top_n_for_source == 3

    # New values populated by stages
    assert final_ctx.changed_symbols is not None
    assert final_ctx.graph is not None
    assert final_ctx.blast_radius is not None


@pytest.mark.asyncio
async def test_pipeline_mcp_calls_receive_correct_arguments(mock_mcp_client):
    """Verify the pipeline passes correct arguments to MCP client methods."""
    pipeline = Pipeline(mock_mcp_client, llm_preference="none")

    ctx = PipelineContext(
        repo_path="/my/repo",
        ref_range="HEAD~2..HEAD",
        max_depth=4
    )

    await pipeline.run(ctx)

    # Verify MCP methods were called with expected arguments
    mock_mcp_client.get_changed_symbols.assert_called_once_with("HEAD~2..HEAD")
    mock_mcp_client.get_dependency_graph.assert_called_once()
    mock_mcp_client.compute_blast_radius.assert_called_once_with("HEAD~2..HEAD", 4)


@pytest.mark.asyncio
async def test_pipeline_generation_outcome_records_provider_and_attempts(mock_mcp_client):
    """Verify GenerationOutcome tracks which provider succeeded and how many attempts."""
    pipeline = Pipeline(mock_mcp_client, llm_preference="none")
    ctx = PipelineContext(repo_path="/fake", ref_range="HEAD~1", max_depth=3)

    result_ctx = await pipeline.run(ctx)

    assert result_ctx.generation_outcome is not None
    assert result_ctx.generation_outcome.provider == "template-only"
    assert result_ctx.generation_outcome.attempts >= 1
    assert isinstance(result_ctx.generation_outcome.degraded, bool)


class _UnknownNodeNarrativeProvider:
    """Simulates an LLM that keeps writing a narrative for a node not in the diagram.

    Goes through spec_builder.parse_narratives, so the real validator raises
    exactly as it would on a real provider's JSON.
    """
    name = "bad-llm"

    def __init__(self):
        self.calls = []

    async def generate_narratives(self, spec, blast_radius, graph, source_snippets, previous_error=None):
        self.calls.append(previous_error)
        payload = {"narratives": [{"node_id": "n_ghost", "narrative": "made up", "risk_reasons": []}]}
        return parse_narratives(json.dumps(payload), spec, blast_radius)


class _ApiFailingProvider:
    name = "flaky-llm"

    def __init__(self):
        self.calls = 0

    async def generate_narratives(self, spec, blast_radius, graph, source_snippets, previous_error=None):
        self.calls += 1
        raise ProviderTransientError("Gemini API 429 RESOURCE_EXHAUSTED: quota")


@pytest.mark.asyncio
async def test_unknown_narrative_node_retries_twice_then_degrades_to_template(mock_mcp_client):
    bad = _UnknownNodeNarrativeProvider()
    pipeline = Pipeline(mock_mcp_client, llm_preference="gemini")
    ctx = PipelineContext(repo_path="/fake", ref_range="HEAD~1", max_depth=3)

    with patch("orchestrator.pipeline.resolve_provider", new=AsyncMock(return_value=bad)):
        result_ctx = await pipeline.run(ctx)

    # One initial attempt plus MAX_RETRIES (2) retries.
    assert MAX_RETRIES == 2
    assert len(bad.calls) == 3
    assert bad.calls[0] is None
    assert all("unknown node" in err for err in bad.calls[1:]), "validation error must be fed back"

    outcome = result_ctx.generation_outcome
    assert outcome.provider == "template-only"
    assert outcome.attempts == 3
    assert outcome.degraded is True
    # Narratives fall back to the template; the hallucinated node never appears.
    assert "ghost" not in result_ctx.mermaid_text
    assert [n.node_id for n in result_ctx.diagram_spec.narratives] == ["n_sym1"]
    assert "n_sym1" in result_ctx.mermaid_text


@pytest.mark.asyncio
async def test_provider_api_error_retries_twice_then_degrades_to_template(mock_mcp_client):
    flaky = _ApiFailingProvider()
    pipeline = Pipeline(mock_mcp_client, llm_preference="gemini")
    ctx = PipelineContext(repo_path="/fake", ref_range="HEAD~1", max_depth=3)

    with patch("orchestrator.pipeline.resolve_provider", new=AsyncMock(return_value=flaky)):
        result_ctx = await pipeline.run(ctx)

    assert flaky.calls == 3
    assert result_ctx.generation_outcome.provider == "template-only"
    assert result_ctx.generation_outcome.degraded is True


@pytest.mark.asyncio
async def test_gemini_api_error_is_raised_as_transient(monkeypatch):
    from google.genai import errors as genai_errors
    from orchestrator.llm.gemini_provider import GeminiProvider

    provider = GeminiProvider(api_key="fake-key", model="gemini-retired")
    retired = genai_errors.ClientError(
        404, {"error": {"code": 404, "message": "model retired", "status": "NOT_FOUND"}})
    monkeypatch.setattr(provider._client.aio.models, "generate_content", AsyncMock(side_effect=retired))
    br = BlastRadiusResult(RootSymbols=[], AffectedNodes=[], MaxDepthUsed=3,
                           TotalNodesAffected=0, TotalEdgesTraversed=0)
    graph = Graph(Nodes={})

    with pytest.raises(ProviderTransientError, match="404 NOT_FOUND: model retired"):
        await provider.generate_narratives(build_spec(br, graph), br, graph, {})


@pytest.mark.asyncio
async def test_degrade_logs_provider_attempt_and_error_at_warning(mock_mcp_client, caplog):
    bad = _UnknownNodeNarrativeProvider()
    pipeline = Pipeline(mock_mcp_client, llm_preference="gemini")
    ctx = PipelineContext(repo_path="/fake", ref_range="HEAD~1", max_depth=3)

    with caplog.at_level("WARNING", logger="blastradius"), \
            patch("orchestrator.pipeline.resolve_provider", new=AsyncMock(return_value=bad)):
        await pipeline.run(ctx)

    warnings = [r.getMessage() for r in caplog.records if r.levelname == "WARNING"]
    assert [m for m in warnings if "attempt 1/3 validation failed" in m and "[bad-llm]" in m]
    degrade = [m for m in warnings if "degrading to template-only" in m]
    assert len(degrade) == 1
    assert "[bad-llm]" in degrade[0]
    assert "(attempt 3)" in degrade[0]
    assert "unknown node: n_ghost" in degrade[0]
    assert "\n" not in degrade[0]
