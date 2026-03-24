"""
LimyeDB Data Models
"""

from typing import List, Dict, Any, Optional, Union
from pydantic import BaseModel, Field


class HNSWConfig(BaseModel):
    """HNSW index configuration."""
    m: Optional[int] = Field(default=16, description="Max connections per node")
    ef_construction: Optional[int] = Field(default=200, description="Index build quality")
    ef_search: Optional[int] = Field(default=100, description="Search quality")


class CollectionConfig(BaseModel):
    """Collection configuration."""
    name: str
    dimension: int
    metric: Optional[str] = Field(default="cosine", description="cosine, euclidean, dot_product")
    on_disk: Optional[bool] = Field(default=False, description="Store vectors on disk")
    hnsw: Optional[HNSWConfig] = None
    replication_factor: Optional[int] = Field(default=1, description="Number of replicas")
    write_consistency_factor: Optional[int] = Field(default=1, description="Write consistency")


class SparseVector(BaseModel):
    """Sparse vector representation mapping indices to weights."""
    indices: List[int]
    values: List[float]


class Point(BaseModel):
    """Vector point with optional payload."""
    id: str
    vector: Optional[List[float]] = None
    named_vectors: Optional[Dict[str, List[float]]] = None
    sparse: Optional[SparseVector] = None
    payload: Optional[Dict[str, Any]] = None


class SearchParams(BaseModel):
    """Search parameters."""
    vector: Optional[List[float]] = None
    query_name: Optional[str] = None
    limit: Optional[int] = 10
    ef: Optional[int] = 100
    filter: Optional[Dict[str, Any]] = None
    with_vector: Optional[bool] = False
    with_payload: Optional[bool] = True
    score_threshold: Optional[float] = None


class Match(BaseModel):
    """Search result match."""
    id: str
    score: float
    vector: Optional[List[float]] = None
    named_vectors: Optional[Dict[str, List[float]]] = None
    sparse: Optional[SparseVector] = None
    payload: Optional[Dict[str, Any]] = None


class MatchCondition(BaseModel):
    """Match condition for filtering."""
    key: str
    match: Dict[str, Any]


class RangeCondition(BaseModel):
    """Range condition for filtering."""
    key: str
    range: Dict[str, float]  # gt, gte, lt, lte


class Filter(BaseModel):
    """Filter conditions."""
    must: Optional[List[Union[MatchCondition, RangeCondition, Dict]]] = None
    must_not: Optional[List[Union[MatchCondition, RangeCondition, Dict]]] = None
    should: Optional[List[Union[MatchCondition, RangeCondition, Dict]]] = None


class ContextExample(BaseModel):
    """Context example for discovery."""
    id: Optional[str] = None
    vector: Optional[List[float]] = None
    named_vectors: Optional[Dict[str, List[float]]] = None


class ContextPair(BaseModel):
    """Positive and negative context for discovery."""
    positive: List[ContextExample]
    negative: Optional[List[ContextExample]] = None


class DiscoverParams(BaseModel):
    """Discovery search parameters."""
    target: Optional[List[float]] = None
    context: Optional[ContextPair] = None
    limit: Optional[int] = 10
    ef: Optional[int] = 100
    filter: Optional[Dict[str, Any]] = None
    with_vector: Optional[bool] = False
    with_payload: Optional[bool] = True


class HybridSearchParams(BaseModel):
    """Hybrid search parameters."""
    dense_vector: Optional[List[float]] = None
    sparse_query: Optional[SparseVector] = None
    limit: Optional[int] = 10
    filter: Optional[Dict[str, Any]] = None
    fusion_method: Optional[str] = "rrf"
    fusion_k: Optional[int] = 60
    with_payload: Optional[bool] = True


class CollectionInfo(BaseModel):
    """Collection information."""
    name: str
    dimension: int
    metric: str
    points_count: int = 0
    status: str = "green"
    config: Optional[CollectionConfig] = None
