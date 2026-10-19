from unittest.mock import AsyncMock, patch

from ollama._types import ListResponse

from orchestrator.llm.ollama_provider import OllamaProvider
from orchestrator.mcp_client import BlastRadiusMCPClient


def _list_response(*tags: str) -> ListResponse:
    return ListResponse(models=[ListResponse.Model(model=t) for t in tags])


async def test_ollama_health_check_matches_model_in_real_list_response_shape():
    provider = OllamaProvider(model="qwen2.5-coder:7b")
    with patch.object(provider._client, "list", new=AsyncMock(return_value=_list_response("qwen2.5-coder:7b"))):
        assert await provider.health_check() is True


async def test_ollama_health_check_false_when_model_not_pulled():
    provider = OllamaProvider(model="qwen2.5-coder:7b")
    with patch.object(provider._client, "list", new=AsyncMock(return_value=_list_response("llama3:8b"))):
        assert await provider.health_check() is False


def test_ollama_reads_model_and_host_from_env(monkeypatch):
    monkeypatch.setenv("OLLAMA_MODEL", "llama3:8b")
    monkeypatch.setenv("OLLAMA_HOST", "http://ollama.internal:9999")
    provider = OllamaProvider()

    assert provider._model == "llama3:8b"
    assert provider.name == "ollama/llama3:8b"
    assert str(provider._client._client.base_url).startswith("http://ollama.internal:9999")


async def test_mcp_client_forwards_cache_db_env_to_go_server(monkeypatch):
    monkeypatch.setenv("BLASTRADIUS_CACHE_DB", "/tmp/custom-cache.db")
    captured = {}

    def fake_stdio_client(params):
        captured["params"] = params
        raise RuntimeError("stop before spawning")

    with patch("orchestrator.mcp_client.stdio_client", side_effect=fake_stdio_client):
        try:
            async with BlastRadiusMCPClient("/bin/blastradius", "/repo"):
                pass
        except RuntimeError:
            pass

    env = captured["params"].env
    assert env["BLASTRADIUS_CACHE_DB"] == "/tmp/custom-cache.db"
    assert "PATH" in env
