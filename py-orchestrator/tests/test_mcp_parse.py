"""_parse against real mcp.types.CallToolResult objects, not mocks.

The pipeline tests mock BlastRadiusMCPClient wholesale, so they never touch the
SDK's result type. mcp 2.x renamed CallToolResult.isError to is_error (isError
is only the wire alias now), which crashed `make demo` on a fresh install while
every test passed. These build the real type from whatever mcp is installed.
Constructing with isError= works on both 1.x (field name) and 2.x (alias).
"""
import pytest
from mcp.types import CallToolResult, TextContent

from orchestrator.errors import MCPToolError
from orchestrator.mcp_client import BlastRadiusMCPClient
from orchestrator.models.graph_types import SymbolSourceResult


def _client():
    return BlastRadiusMCPClient("/bin/blastradius", "/repo")


def test_parse_success_validates_text_payload():
    payload = SymbolSourceResult(symbol_id="a.py::f", source="def f(): pass", truncated=False)
    result = CallToolResult(content=[TextContent(type="text", text=payload.model_dump_json())])

    assert _client()._parse(result, SymbolSourceResult) == payload


def test_parse_error_raises_mcp_tool_error_with_message():
    result = CallToolResult(
        content=[TextContent(type="text", text="repo_path does not exist")], isError=True
    )

    with pytest.raises(MCPToolError, match="repo_path does not exist"):
        _client()._parse(result, SymbolSourceResult)


def test_parse_empty_content_raises_mcp_tool_error():
    result = CallToolResult(content=[])

    with pytest.raises(MCPToolError, match="no content"):
        _client()._parse(result, SymbolSourceResult)
