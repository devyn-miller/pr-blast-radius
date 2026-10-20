import os
from contextlib import AsyncExitStack
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import get_default_environment, stdio_client
from pydantic import TypeAdapter

from orchestrator.models.graph_types import (
    ChangedSymbol, Graph, BlastRadiusResult, SymbolSourceResult
)
from orchestrator.errors import MCPToolError


class BlastRadiusMCPClient:
    def __init__(self, go_binary_path: str, repo_path: str):
        self._binary_path = go_binary_path
        self._repo_path = repo_path
        self._stack = AsyncExitStack()
        self._session: ClientSession | None = None

    async def __aenter__(self) -> "BlastRadiusMCPClient":
        # stdio_client only forwards a small allowlist of env vars (HOME,
        # PATH, ...), so anything the Go server reads must be passed explicitly.
        env = get_default_environment()
        if cache_db := os.getenv("BLASTRADIUS_CACHE_DB"):
            env["BLASTRADIUS_CACHE_DB"] = cache_db
        params = StdioServerParameters(command=self._binary_path, args=["mcp-serve"], env=env)
        read, write = await self._stack.enter_async_context(stdio_client(params))
        self._session = await self._stack.enter_async_context(ClientSession(read, write))
        await self._session.initialize()
        return self

    async def __aexit__(self, *exc):
        await self._stack.aclose()

    async def get_changed_symbols(self, ref_range: str) -> list[ChangedSymbol]:
        result = await self._session.call_tool(
            "get_changed_symbols", {"repo_path": self._repo_path, "ref_range": ref_range}
        )
        return self._parse(result, list[ChangedSymbol])

    async def get_dependency_graph(self) -> Graph:
        result = await self._session.call_tool(
            "get_dependency_graph", {"repo_path": self._repo_path}
        )
        return self._parse(result, Graph)

    async def compute_blast_radius(self, ref_range: str, max_depth: int = 3) -> BlastRadiusResult:
        result = await self._session.call_tool(
            "compute_blast_radius",
            {"repo_path": self._repo_path, "ref_range": ref_range, "max_depth": max_depth},
        )
        return self._parse(result, BlastRadiusResult)

    async def get_symbol_source(self, symbol_id: str, max_bytes: int = 2000) -> SymbolSourceResult:
        result = await self._session.call_tool(
            "get_symbol_source",
            {"repo_path": self._repo_path, "symbol_id": symbol_id, "max_bytes": max_bytes},
        )
        return self._parse(result, SymbolSourceResult)

    def _parse(self, result, model):
        # mcp 2.x renamed CallToolResult.isError to is_error ("isError" is now
        # only the wire alias); 1.x has only isError. Read whichever exists.
        is_error = getattr(result, "is_error", None)
        if is_error is None:
            is_error = result.isError
        if is_error:
            raise MCPToolError(" ".join(getattr(c, "text", repr(c)) for c in result.content))
        if not result.content:
            raise MCPToolError("MCP tool returned no content")
        first = result.content[0]
        if getattr(first, "text", None) is None:
            raise MCPToolError(f"MCP tool returned {first.type!r} content, expected text")
        return TypeAdapter(model).validate_json(first.text)