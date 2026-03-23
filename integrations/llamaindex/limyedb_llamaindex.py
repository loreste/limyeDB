"""
LimyeDB LlamaIndex Integration

This module provides a LlamaIndex VectorStore implementation for LimyeDB.

Example:
    >>> from llama_index.core import VectorStoreIndex, SimpleDirectoryReader
    >>> from limyedb_llamaindex import LimyeDBVectorStore
    >>>
    >>> # Create vector store
    >>> vector_store = LimyeDBVectorStore(
    ...     url="http://localhost:8080",
    ...     api_key="your-api-key",
    ...     collection_name="documents"
    ... )
    >>>
    >>> # Create index
    >>> index = VectorStoreIndex.from_vector_store(vector_store)
    >>>
    >>> # Query
    >>> query_engine = index.as_query_engine()
    >>> response = query_engine.query("What is LimyeDB?")
"""

from typing import Any, Dict, List, Optional
import uuid

try:
    from llama_index.core.vector_stores.types import (
        VectorStore,
        VectorStoreQuery,
        VectorStoreQueryResult,
    )
    from llama_index.core.schema import TextNode, BaseNode
except ImportError:
    from llama_index.vector_stores.types import (
        VectorStore,
        VectorStoreQuery,
        VectorStoreQueryResult,
    )
    from llama_index.schema import TextNode, BaseNode

import requests


class LimyeDBVectorStore(VectorStore):
    """
    LimyeDB Vector Store for LlamaIndex.

    Args:
        url: LimyeDB server URL
        api_key: API key for authentication
        collection_name: Name of the collection
        dimension: Vector dimension (auto-detected if not provided)
    """

    stores_text: bool = True
    flat_metadata: bool = True

    def __init__(
        self,
        url: str = "http://localhost:8080",
        api_key: Optional[str] = None,
        collection_name: str = "llamaindex",
        dimension: Optional[int] = None,
        distance_strategy: str = "cosine",
        **kwargs: Any,
    ):
        self.url = url.rstrip("/")
        self.api_key = api_key
        self.collection_name = collection_name
        self.dimension = dimension
        self.distance_strategy = distance_strategy

        self.session = requests.Session()
        self.session.headers["Content-Type"] = "application/json"
        if api_key:
            self.session.headers["Authorization"] = f"Bearer {api_key}"

    def _request(
        self,
        method: str,
        path: str,
        json: Optional[Dict] = None
    ) -> Dict:
        """Make HTTP request to LimyeDB."""
        resp = self.session.request(
            method=method,
            url=f"{self.url}{path}",
            json=json,
            timeout=30
        )
        resp.raise_for_status()
        return resp.json() if resp.text else {}

    def _ensure_collection(self, dimension: int) -> None:
        """Ensure the collection exists."""
        try:
            self._request("GET", f"/collections/{self.collection_name}")
        except requests.HTTPError as e:
            if e.response.status_code == 404:
                self._request("POST", "/collections", {
                    "name": self.collection_name,
                    "dimension": dimension,
                    "metric": self.distance_strategy
                })
            else:
                raise

    @property
    def client(self) -> Any:
        """Return the underlying client."""
        return self.session

    def add(
        self,
        nodes: List[BaseNode],
        **add_kwargs: Any,
    ) -> List[str]:
        """
        Add nodes to the vector store.

        Args:
            nodes: List of nodes to add

        Returns:
            List of node IDs
        """
        if not nodes:
            return []

        first_embedding = nodes[0].get_embedding()
        if first_embedding:
            self._ensure_collection(len(first_embedding))
        elif self.dimension:
            self._ensure_collection(self.dimension)
        else:
            raise ValueError("Cannot determine embedding dimension")

        points = []
        ids = []

        for node in nodes:
            node_id = node.node_id or str(uuid.uuid4())
            ids.append(node_id)

            embedding = node.get_embedding()
            if embedding is None:
                continue

            payload = {
                "text": node.get_content(),
                "doc_id": node.ref_doc_id or "",
            }

            if hasattr(node, "metadata") and node.metadata:
                payload["metadata"] = node.metadata
                for key, value in node.metadata.items():
                    if isinstance(value, (str, int, float, bool)):
                        payload[key] = value

            points.append({
                "id": node_id,
                "vector": embedding,
                "payload": payload
            })

        if points:
            self._request(
                "PUT",
                f"/collections/{self.collection_name}/points",
                {"points": points}
            )

        return ids

    def delete(self, ref_doc_id: str, **delete_kwargs: Any) -> None:
        """
        Delete nodes by reference document ID.

        Args:
            ref_doc_id: Reference document ID
        """
        result = self._request(
            "POST",
            f"/collections/{self.collection_name}/points/scroll",
            {
                "filter": {"doc_id": ref_doc_id},
                "limit": 10000,
                "with_payload": False,
                "with_vector": False
            }
        )

        ids = [p["id"] for p in result.get("points", [])]
        if ids:
            self._request(
                "POST",
                f"/collections/{self.collection_name}/points/delete",
                {"ids": ids}
            )

    def query(
        self,
        query: VectorStoreQuery,
        **kwargs: Any,
    ) -> VectorStoreQueryResult:
        """
        Query the vector store.

        Args:
            query: VectorStoreQuery with embedding

        Returns:
            VectorStoreQueryResult with nodes and scores
        """
        if query.query_embedding is None:
            return VectorStoreQueryResult(nodes=[], similarities=[], ids=[])

        payload = {
            "vector": query.query_embedding,
            "limit": query.similarity_top_k or 10,
            "with_payload": True,
            "with_vector": False
        }

        if query.filters:
            payload["filter"] = self._build_filter(query.filters)

        result = self._request(
            "POST",
            f"/collections/{self.collection_name}/search",
            payload
        )

        nodes = []
        scores = []
        ids = []

        for hit in result.get("result", []):
            payload = hit.get("payload", {})
            text = payload.get("text", "")
            metadata = payload.get("metadata", {})

            for key, value in payload.items():
                if key not in ("text", "metadata", "doc_id"):
                    metadata[key] = value

            node = TextNode(
                text=text,
                id_=hit["id"],
                metadata=metadata
            )

            if "doc_id" in payload:
                node.ref_doc_id = payload["doc_id"]

            nodes.append(node)
            scores.append(hit.get("score", 0.0))
            ids.append(hit["id"])

        return VectorStoreQueryResult(
            nodes=nodes,
            similarities=scores,
            ids=ids
        )

    def _build_filter(self, filters: Any) -> Dict:
        """Build filter from LlamaIndex filters."""
        if filters is None:
            return {}

        if hasattr(filters, "legacy_filters"):
            filters = filters.legacy_filters()

        if hasattr(filters, "filters"):
            conditions = []
            for f in filters.filters:
                key = f.key
                value = f.value
                operator = getattr(f, "operator", "==")

                if operator == "==":
                    conditions.append({key: value})
                elif operator == "!=":
                    conditions.append({key: {"$ne": value}})
                elif operator == ">":
                    conditions.append({key: {"$gt": value}})
                elif operator == ">=":
                    conditions.append({key: {"$gte": value}})
                elif operator == "<":
                    conditions.append({key: {"$lt": value}})
                elif operator == "<=":
                    conditions.append({key: {"$lte": value}})
                elif operator == "in":
                    conditions.append({key: {"$in": value}})

            if conditions:
                return {"$and": conditions}

        return {}


def create_limyedb_index(
    documents: List[Any],
    url: str = "http://localhost:8080",
    api_key: Optional[str] = None,
    collection_name: str = "llamaindex",
    embed_model: Optional[Any] = None,
    **kwargs: Any,
) -> Any:
    """
    Helper function to create a LlamaIndex VectorStoreIndex with LimyeDB.

    Args:
        documents: List of documents to index
        url: LimyeDB URL
        api_key: API key
        collection_name: Collection name
        embed_model: Embedding model

    Returns:
        VectorStoreIndex
    """
    try:
        from llama_index.core import VectorStoreIndex, StorageContext
    except ImportError:
        from llama_index import VectorStoreIndex, StorageContext

    vector_store = LimyeDBVectorStore(
        url=url,
        api_key=api_key,
        collection_name=collection_name,
        **kwargs
    )

    storage_context = StorageContext.from_defaults(vector_store=vector_store)

    index_kwargs = {"storage_context": storage_context}
    if embed_model:
        index_kwargs["embed_model"] = embed_model

    return VectorStoreIndex.from_documents(documents, **index_kwargs)
