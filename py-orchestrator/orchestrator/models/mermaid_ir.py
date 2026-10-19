from pydantic import BaseModel, Field, ValidationInfo, field_validator
from typing import Literal
import re

class SubgraphSpec(BaseModel):
    id: str = Field(pattern=r"^[A-Za-z0-9_]+$")
    title: str
    node_ids: list[str]

class NodeStyle(BaseModel):
    id: str = Field(pattern=r"^[A-Za-z0-9_]+$")
    label: str = Field(max_length=60)
    # "changed" marks a root symbol from the diff (it has no severity score).
    risk_tier: Literal["changed", "critical", "high", "medium", "low"]

    @field_validator("label")
    @classmethod
    def strip_mermaid_breaking_chars(cls, v: str) -> str:
        return re.sub(r'[|{}"\[\]]', "", v)

class EdgeSpec(BaseModel):
    from_id: str
    to_id: str
    style: Literal["solid", "dashed"] = "solid"

class NarrativeItem(BaseModel):
    """A narrative as RENDERED: the LLM's prose plus Go's own risk reasons.

    risk_reasons is never taken from a provider's output — spec_builder fills
    it from BlastRadiusResult.SeverityReasons. See LLMNarrative.
    """
    node_id: str
    narrative: str = Field(max_length=280)
    risk_reasons: list[str]

# risk_reasons is deliberately absent from LLMNarrative. It used to be part of
# this schema, validated against Go's reasons after the fact, which still left
# the model free to drop or reorder them as long as it invented none. With no
# field at all the attack surface is gone rather than guarded: there is nothing
# for a provider to fill, and Pydantic drops it if one sends it anyway.
# spec_builder.parse_narratives fills risk_reasons from Go's SeverityReasons.
#
# Docstrings on these two classes would be emitted as JSON-schema
# `description`s and shipped to the provider, so the reasoning stays in
# comments and the schema stays minimal.
class LLMNarrative(BaseModel):
    node_id: str
    narrative: str = Field(max_length=280)

class NarrativeSpec(BaseModel):
    """The only thing an LLM produces: narratives for already-placed nodes.

    This is the schema handed to the providers (Gemini's response_schema,
    Ollama's `format`), so narrowing it narrows what they can return.

    Validated with context from spec_builder.parse_narratives:
      node_ids: Mermaid node IDs present in the built spec
    Without that context (e.g. JSON-schema generation) only shape is checked.
    """
    narratives: list[LLMNarrative]

    @field_validator("narratives")
    @classmethod
    def narratives_reference_built_nodes(cls, narratives, info: ValidationInfo):
        ctx = info.context or {}
        node_ids = ctx.get("node_ids")
        if node_ids is None:
            return narratives
        for item in narratives:
            if item.node_id not in node_ids:
                raise ValueError(f"Narrative references unknown node: {item.node_id}")
        return narratives

class MermaidDiagramSpec(BaseModel):
    direction: Literal["TD", "LR"] = "TD"
    subgraphs: list[SubgraphSpec]
    nodes: list[NodeStyle]
    edges: list[EdgeSpec]
    narratives: list[NarrativeItem]

    @field_validator("edges")
    @classmethod
    def edges_reference_known_nodes(cls, edges, info):
        node_ids = {n.id for n in info.data.get("nodes", [])}
        for e in edges:
            if e.from_id not in node_ids or e.to_id not in node_ids:
                raise ValueError(f"Edge references unknown node: {e.from_id}->{e.to_id}")
        return edges