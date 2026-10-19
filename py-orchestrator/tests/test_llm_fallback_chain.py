# py-orchestrator/tests/test_llm_fallback_chain.py
import pytest
from unittest.mock import AsyncMock, patch
from orchestrator.llm.resolve import resolve_provider
from orchestrator.llm.template_provider import TemplateOnlyProvider
from orchestrator.errors import ProviderUnavailableError
from orchestrator.llm.gemini_provider import DEFAULT_MODEL


@pytest.mark.asyncio
async def test_auto_falls_through_to_template_when_nothing_available(monkeypatch):
    monkeypatch.delenv("GEMINI_API_KEY", raising=False)
    with patch("orchestrator.llm.ollama_provider.OllamaProvider.health_check", new=AsyncMock(return_value=False)):
        provider = await resolve_provider("auto")
    assert isinstance(provider, TemplateOnlyProvider)


@pytest.mark.asyncio
async def test_auto_prefers_gemini_when_key_present_and_healthy(monkeypatch):
    monkeypatch.setenv("GEMINI_API_KEY", "fake-key")
    with patch("orchestrator.llm.gemini_provider.GeminiProvider.health_check", new=AsyncMock(return_value=True)):
        provider = await resolve_provider("auto")
    assert provider.name == DEFAULT_MODEL


@pytest.mark.asyncio
async def test_explicit_gemini_raises_if_unhealthy(monkeypatch):
    monkeypatch.setenv("GEMINI_API_KEY", "fake-key")
    with patch("orchestrator.llm.gemini_provider.GeminiProvider.health_check", new=AsyncMock(return_value=False)):
        with pytest.raises(ProviderUnavailableError):
            await resolve_provider("gemini")


@pytest.mark.asyncio
async def test_none_always_returns_template_provider():
    provider = await resolve_provider("none")
    assert isinstance(provider, TemplateOnlyProvider)
    assert await provider.health_check() is True