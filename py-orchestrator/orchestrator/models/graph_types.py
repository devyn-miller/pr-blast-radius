from pydantic import BaseModel, Field
from datetime import datetime

class ChangedSymbol(BaseModel):
    SymbolID: str
    ChangeType: str  # "Added" | "Modified" | "Deleted" | "SignatureChanged"

class GraphMetadata(BaseModel):
    RepoPath: str = ""
    CommitSHA: str = ""
    GeneratedAt: datetime | None = None
    ToolVersion: str = ""
    CacheHitRatio: float = 0.0
    UnresolvedCallCount: int = 0

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
    Tags: list[str] | None = None

class CallSite(BaseModel):
    File: str = ""
    Line: int = 0

class Edge(BaseModel):
    From: str
    To: str
    Type: str
    Confidence: float
    Tags: list[str] | None = None
    Site: CallSite = Field(default_factory=CallSite)

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
    ExecutionTime: int = 0  # Go time.Duration: integer nanoseconds
    # Changed files that are gone from disk, so they contribute no nodes to the
    # graph and nothing in them can appear as a root or as affected.
    DeletedFilesNotAnalyzed: list[str] = Field(default_factory=list)

class SymbolSourceResult(BaseModel):
    symbol_id: str
    source: str
    truncated: bool