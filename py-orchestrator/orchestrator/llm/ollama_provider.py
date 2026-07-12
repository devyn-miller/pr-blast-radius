import ollama
from orchestrator.models.mermaid_ir import MermaidDiagramSpec
from orchestrator.prompts import render_prompt

class OllamaProvider:
    name = "ollama/qwen2.5-coder:7b"

    def __init__(self, model: str = "qwen2.5-coder:7b", host: str = "http://localhost:11434"):
        self._client = ollama.AsyncClient(host=host)
        self._model = model

    async def health_check(self) -> bool:
        try:
            models = await self._client.list()
            return any(self._model in m["name"] for m in models.get("models", []))
        except Exception:
            return False

    async def generate_diagram_spec(self, blast_radius, graph, source_snippets, previous_error=None):
        prompt = render_prompt("mermaid_spec.j2", blast_radius=blast_radius, graph=graph,
                                source_snippets=source_snippets, previous_error=previous_error)
        response = await self._client.chat(
            model=self._model, messages=[{"role": "user", "content": prompt}],
            format=MermaidDiagramSpec.model_json_schema(), options={"temperature": 0.2},
        )
        return MermaidDiagramSpec.model_validate_json(response["message"]["content"])