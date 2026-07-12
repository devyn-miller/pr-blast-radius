import os
from orchestrator.llm.gemini_provider import GeminiProvider
from orchestrator.llm.ollama_provider import OllamaProvider
from orchestrator.llm.template_provider import TemplateOnlyProvider
from orchestrator.errors import ProviderUnavailableError

async def resolve_provider(preference: str = "auto"):
    if preference == "none":
        return TemplateOnlyProvider()

    if preference in ("auto", "gemini"):
        api_key = os.getenv("GEMINI_API_KEY")
        if api_key:
            provider = GeminiProvider(api_key=api_key)
            if await provider.health_check():
                return provider
            if preference == "gemini":
                raise ProviderUnavailableError("GEMINI_API_KEY set but health check failed")

    if preference in ("auto", "ollama"):
        provider = OllamaProvider()
        if await provider.health_check():
            return provider
        if preference == "ollama":
            raise ProviderUnavailableError("Ollama daemon unreachable or model not pulled")

    if preference == "auto":
        return TemplateOnlyProvider()

    raise ValueError(f"Unknown provider preference: {preference}")