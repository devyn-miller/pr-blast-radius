from pydantic import BaseModel, Field
from datetime import datetime

class ChangedSymbol(BaseModel):
    symbol_id: str
    change_type: str  # "Added" | "Modified" | "Deleted" | "SignatureChanged"

class GraphMetadata(BaseModel):
    repo_path: str = ""
    commit_sha: str = ""
    generated_at: datetime | None = None
    tool_version: str = ""
    cache_hit_ratio: float = 0.0
    unresolved_call_count: int = 0

class Node(BaseModel):
    ID: str
    Kind: str
    FilePath: str
    Name: str
    QualifiedName: str
    Language: str
    Exported: bool
    StartLine: int
    EndLine: int
    InDegree: int = 0
    OutDegree: int = 0
    Tags: list[str] = Field(default_factory=list)

class Edge(BaseModel):
    From: str
    To: str
    Type: str
    Confidence: float
    Tags: list[str] = Field(default_factory=list)

class Graph(BaseModel):
    Nodes: dict[str, Node]
    OutEdges: dict[str, list[Edge]] = Field(default_factory=dict)
    InEdges: dict[str, list[Edge]] = Field(default_factory=dict)
    Metadata: GraphMetadata = Field(default_factory=GraphMetadata)

class AffectedNode(BaseModel):
    NodeID: str
    Distance: int
    Path: list[str]
    SeverityScore: float
    SeverityReasons: list[str] = Field(default_factory=list)

class BlastRadiusResult(BaseModel):
    RootSymbols: list[ChangedSymbol]
    AffectedNodes: list[AffectedNode]
    MaxDepthUsed: int
    TotalNodesAffected: int
    TotalEdgesTraversed: int

class SymbolSourceResult(BaseModel):
    symbol_id: str
    source: str
    truncated: bool