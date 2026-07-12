from pydantic import BaseModel, Field, field_validator
from typing import Literal
import re

class SubgraphSpec(BaseModel):
    id: str = Field(pattern=r"^[A-Za-z0-9_]+$")
    title: str
    node_ids: list[str]

class NodeStyle(BaseModel):
    id: str = Field(pattern=r"^[A-Za-z0-9_]+$")
    label: str = Field(max_length=60)
    risk_tier: Literal["critical", "high", "medium", "low"]

    @field_validator("label")
    @classmethod
    def strip_mermaid_breaking_chars(cls, v: str) -> str:
        return re.sub(r'[|{}"\[\]]', "", v)

class EdgeSpec(BaseModel):
    from_id: str
    to_id: str
    style: Literal["solid", "dashed"] = "solid"

class NarrativeItem(BaseModel):
    node_id: str
    narrative: str = Field(max_length=280)
    risk_reasons: list[str]

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