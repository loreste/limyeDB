"""
LimyeDB Python Client

Official Python client for LimyeDB - Enterprise Distributed Vector Database for AI & RAG.

Example:
    >>> from limyedb import LimyeDBClient, Point, CollectionConfig
    >>>
    >>> client = LimyeDBClient(host="http://localhost:8080", api_key="your-key")
    >>> client.create_collection(name="docs", dimension=1536, metric="cosine")
    >>> client.upsert("docs", [Point(id="1", vector=[0.1, 0.2, ...], payload={"text": "Hello"})])
    >>> results = client.search("docs", vector=[0.1, 0.2, ...], limit=10)
"""

from .client import (
    LimyeDBClient,
    LimyeDBError,
    ConnectionError,
    AuthenticationError,
    CollectionNotFoundError,
)
from .models import (
    Point,
    CollectionConfig,
    SearchParams,
    Match,
    HNSWConfig,
    Filter,
    MatchCondition,
    RangeCondition,
    ContextExample,
    ContextPair,
    DiscoverParams,
    HybridSearchParams,
    CollectionInfo,
)

__version__ = "1.0.0"
__all__ = [
    # Client
    "LimyeDBClient",
    # Exceptions
    "LimyeDBError",
    "ConnectionError",
    "AuthenticationError",
    "CollectionNotFoundError",
    # Models
    "Point",
    "CollectionConfig",
    "SearchParams",
    "Match",
    "HNSWConfig",
    "Filter",
    "MatchCondition",
    "RangeCondition",
    "ContextExample",
    "ContextPair",
    "DiscoverParams",
    "HybridSearchParams",
    "CollectionInfo",
]
