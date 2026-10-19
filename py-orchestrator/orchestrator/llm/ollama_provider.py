import os
import ollama
from orchestrator.models.mermaid_ir import NarrativeSpec
from orchestrator.prompts import render_narrative_prompt
from orchestrator.spec_builder import parse_narratives

DEFAULT_MODEL = "qwen2.5-coder:7b"
DEFAULT_HOST = "http://localhost:11434"

class OllamaProvider:
    def __init__(self, model: str | None = None, host: str | None = None):
        self._model = model or os.getenv("OLLAMA_MODEL") or DEFAULT_MODEL
        self._client = ollama.AsyncClient(host=host or os.getenv("OLLAMA_HOST") or DEFAULT_HOST)
        self.name = f"ollama/{self._model}"

    async def health_check(self) -> bool:
        try:
            # ollama>=0.4 returns a ListResponse whose entries expose the tag
            # as `.model`; the old dict-style m["name"] raises KeyError.
            response = await self._client.list()
            return any(self._model in (m.model or "") for m in response.models)
        except Exception:
            return False

    async def generate_narratives(self, spec, blast_radius, graph, source_snippets, previous_error=None):
        prompt = render_narrative_prompt(spec, blast_radius, source_snippets, previous_error)
        response = await self._client.chat(
            model=self._model, messages=[{"role": "user", "content": prompt}],
            format=NarrativeSpec.model_json_schema(), options={"temperature": 0.2},
        )
        return parse_narratives(response["message"]["content"], spec, blast_radius)
