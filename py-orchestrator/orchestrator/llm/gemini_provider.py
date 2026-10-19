import os
from google import genai
from google.genai import errors as genai_errors
from orchestrator.errors import ProviderTransientError
from orchestrator.models.mermaid_ir import NarrativeSpec
from orchestrator.prompts import render_narrative_prompt
from orchestrator.spec_builder import parse_narratives

# gemini-2.5-flash was retired for new API users (404 NOT_FOUND); override
# with GEMINI_MODEL when this one is retired too.
DEFAULT_MODEL = "gemini-3.8-flash"

class GeminiProvider:
    def __init__(self, api_key: str, model: str | None = None):
        self._client = genai.Client(api_key=api_key)
        self._model = model or os.getenv("GEMINI_MODEL") or DEFAULT_MODEL
        self.name = self._model

    async def health_check(self) -> bool:
        # Look up the configured model rather than just listing models, so a
        # mistyped or removed model name fails here and `auto` falls through.
        try:
            await self._client.aio.models.get(model=self._model)
            return True
        except Exception:
            return False

    async def generate_narratives(self, spec, blast_radius, graph, source_snippets, previous_error=None):
        prompt = render_narrative_prompt(spec, blast_radius, source_snippets, previous_error)
        try:
            response = await self._client.aio.models.generate_content(
                model=self._model, contents=prompt,
                config={"response_mime_type": "application/json",
                        "response_schema": NarrativeSpec, "temperature": 0.2},
            )
        except genai_errors.APIError as e:
            # Quota (429), server (5xx) and model/config (4xx) errors all go
            # through the pipeline's retry-then-degrade path instead of
            # crashing the run; a 4xx won't succeed on retry, but degrading
            # to template-only beats aborting with no diagram.
            raise ProviderTransientError(f"Gemini API {e.code} {e.status}: {e.message}") from e
        return parse_narratives(response.text, spec, blast_radius)
