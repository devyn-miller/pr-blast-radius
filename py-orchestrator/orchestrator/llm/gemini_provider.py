from google import genai
from orchestrator.models.mermaid_ir import MermaidDiagramSpec
from orchestrator.prompts import render_prompt

class GeminiProvider:
    name = "gemini-2.5-flash"

    def __init__(self, api_key: str, model: str = "gemini-2.5-flash"):
        self._client = genai.Client(api_key=api_key)
        self._model = model

    async def health_check(self) -> bool:
        try:
            await self._client.aio.models.list()
            return True
        except Exception:
            return False

    async def generate_diagram_spec(self, blast_radius, graph, source_snippets, previous_error=None):
        prompt = render_prompt("mermaid_spec.j2", blast_radius=blast_radius, graph=graph,
                                source_snippets=source_snippets, previous_error=previous_error)
        response = await self._client.aio.models.generate_content(
            model=self._model, contents=prompt,
            config={"response_mime_type": "application/json",
                    "response_schema": MermaidDiagramSpec, "temperature": 0.2},
        )
        return MermaidDiagramSpec.model_validate_json(response.text)