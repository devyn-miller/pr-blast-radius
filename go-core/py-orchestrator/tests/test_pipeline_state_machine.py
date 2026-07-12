# py-orchestrator/tests/test_pipeline_state_machine.py
import pytest
from unittest.mock import AsyncMock
from orchestrator.pipeline import Pipeline, PipelineContext
from orchestrator.errors import NoChangesDetectedError
from orchestrator.models.graph_types import ChangedSymbol


@pytest.mark.asyncio
async def test_pipeline_raises_when_no_changed_symbols():
    mock_client = AsyncMock()
    mock_client.get_changed_symbols.return_value = []

    pipeline = Pipeline(mock_client)
    ctx = PipelineContext(repo_path="/tmp/x", ref_range="HEAD~1", max_depth=3)

    with pytest.raises(NoChangesDetectedError):
        await pipeline._extract_changes(ctx)


@pytest.mark.asyncio
async def test_fetch_top_n_source_only_calls_for_highest_severity_nodes():
    from orchestrator.models.graph_types import BlastRadiusResult, AffectedNode

    mock_client = AsyncMock()
    mock_client.get_symbol_source.return_value.source = "def f(): pass"

    ctx = PipelineContext(repo_path="/tmp/x", ref_range="HEAD~1", max_depth=3, top_n_for_source=1)
    ctx.blast_radius = BlastRadiusResult(
        RootSymbols=[ChangedSymbol(symbol_id="root", change_type="Modified")],
        AffectedNodes=[
            AffectedNode(NodeID="low", Distance=2, Path=["root", "low"], SeverityScore=0.1),
            AffectedNode(NodeID="high", Distance=1, Path=["root", "high"], SeverityScore=0.9),
        ],
        MaxDepthUsed=3, TotalNodesAffected=2, TotalEdgesTraversed=2,
    )

    pipeline = Pipeline(mock_client)
    await pipeline._fetch_top_n_source(ctx)

    mock_client.get_symbol_source.assert_called_once_with("high")
    assert "high" in ctx.source_snippets
    assert "low" not in ctx.source_snippets