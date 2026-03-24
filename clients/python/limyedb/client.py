"""
LimyeDB Python Client - Enhanced Edition
"""

import requests
from typing import List, Dict, Any, Optional, Union
from urllib.parse import urljoin

from .models import Point, CollectionConfig, Match, DiscoverParams, HNSWConfig, Filter, SearchParams, SparseVector


class LimyeDBError(Exception):
    """Base exception for LimyeDB errors."""
    pass


class ConnectionError(LimyeDBError):
    """Failed to connect to LimyeDB server."""
    pass


class AuthenticationError(LimyeDBError):
    """Authentication failed."""
    pass


class CollectionNotFoundError(LimyeDBError):
    """Collection not found."""
    pass


class LimyeDBClient:
    """
    Official Python Client for LimyeDB - Enterprise Distributed Vector Database.

    Example:
        >>> client = LimyeDBClient(host="http://localhost:8080", api_key="your-key")
        >>> client.create_collection(CollectionConfig(name="docs", dimension=1536))
        >>> client.upsert("docs", [Point(id="1", vector=[0.1, 0.2, ...])])
        >>> results = client.search("docs", vector=[0.1, 0.2, ...], limit=10)
    """

    def __init__(
        self,
        host: str = "http://localhost:8080",
        auth_token: Optional[str] = None,
        api_key: Optional[str] = None,
        timeout: int = 30
    ):
        """
        Initialize the LimyeDB client.

        Args:
            host: LimyeDB server URL
            auth_token: JWT Authorization token
            api_key: Legacy API key for authentication (optional)
            timeout: Request timeout in seconds
        """
        self.host = host.rstrip('/')
        self.auth_token = auth_token
        self.api_key = api_key
        self.timeout = timeout
        self.session = requests.Session()

        token = auth_token or api_key
        if token:
            self.session.headers["Authorization"] = f"Bearer {token}"
        self.session.headers["Content-Type"] = "application/json"

    def _request(self, method: str, path: str, json: dict = None) -> dict:
        """Make an HTTP request to the server."""
        try:
            resp = self.session.request(
                method=method,
                url=f"{self.host}{path}",
                json=json,
                timeout=self.timeout
            )
        except requests.exceptions.ConnectionError as e:
            raise ConnectionError(f"Failed to connect to LimyeDB: {e}")
        except requests.exceptions.Timeout as e:
            raise ConnectionError(f"Request timed out: {e}")

        if resp.status_code == 401:
            raise AuthenticationError("Invalid API key")
        elif resp.status_code == 404:
            if "collection" in resp.text.lower():
                raise CollectionNotFoundError(resp.text)
            raise LimyeDBError(f"Not found: {resp.text}")

        resp.raise_for_status()
        return resp.json() if resp.text else {}

    def _post(self, path: str, json: dict = None) -> dict:
        return self._request("POST", path, json)

    def _put(self, path: str, json: dict = None) -> dict:
        return self._request("PUT", path, json)

    def _get(self, path: str) -> dict:
        return self._request("GET", path)

    def _delete(self, path: str) -> dict:
        return self._request("DELETE", path)

    # Collections API

    def create_collection(
        self,
        config: CollectionConfig = None,
        *,
        name: str = None,
        dimension: int = None,
        metric: str = "cosine",
        hnsw_config: HNSWConfig = None
    ) -> dict:
        """
        Create a new collection.

        Args:
            config: CollectionConfig object, or use keyword args
            name: Collection name
            dimension: Vector dimension
            metric: Distance metric (cosine, euclidean, dot_product)
            hnsw_config: HNSW index configuration
        """
        if config:
            return self._post("/collections", json=config.model_dump(exclude_none=True))

        data = {
            "name": name,
            "dimension": dimension,
            "metric": metric,
        }
        if hnsw_config:
            data["hnsw"] = hnsw_config.model_dump(exclude_none=True)
        return self._post("/collections", json=data)

    def list_collections(self) -> dict:
        """List all collections."""
        return self._get("/collections")

    def get_collection(self, name: str) -> dict:
        """Get collection info."""
        return self._get(f"/collections/{name}")

    def delete_collection(self, name: str) -> dict:
        """Delete a collection."""
        return self._delete(f"/collections/{name}")

    def collection_exists(self, name: str) -> bool:
        """Check if a collection exists."""
        try:
            self.get_collection(name)
            return True
        except CollectionNotFoundError:
            return False

    # Vectors API

    def upsert(
        self,
        collection_name: str,
        points: List[Point],
        wait: bool = True
    ) -> dict:
        """
        Upsert points into a collection.

        Args:
            collection_name: Target collection
            points: List of Point objects
            wait: Wait for operation to complete
        """
        payload = {
            "points": [p.model_dump(exclude_none=True) for p in points],
            "wait": wait
        }
        return self._put(f"/collections/{collection_name}/points", json=payload)

    def upsert_batch(
        self,
        collection_name: str,
        points: List[Point],
        batch_size: int = 100
    ) -> List[dict]:
        """
        Upsert points in batches.

        Args:
            collection_name: Target collection
            points: List of Point objects
            batch_size: Number of points per batch
        """
        results = []
        for i in range(0, len(points), batch_size):
            batch = points[i:i + batch_size]
            result = self.upsert(collection_name, batch)
            results.append(result)
        return results

    def delete(self, collection_name: str, ids: List[str]) -> dict:
        """Delete points by IDs."""
        return self._post(f"/collections/{collection_name}/points/delete", json={"ids": ids})

    def get_point(
        self,
        collection_name: str,
        point_id: str,
        with_vector: bool = True,
        with_payload: bool = True
    ) -> dict:
        """Get a single point by ID."""
        params = f"?with_vector={with_vector}&with_payload={with_payload}"
        return self._get(f"/collections/{collection_name}/points/{point_id}{params}")

    def get_points(
        self,
        collection_name: str,
        ids: List[str],
        with_vector: bool = True,
        with_payload: bool = True
    ) -> dict:
        """Get multiple points by IDs."""
        payload = {
            "ids": ids,
            "with_vector": with_vector,
            "with_payload": with_payload
        }
        return self._post(f"/collections/{collection_name}/points/get", json=payload)

    def search(
        self,
        collection_name: str,
        vector: Optional[List[float]] = None,
        query_name: Optional[str] = None,
        limit: int = 10,
        filter: Optional[Dict[str, Any]] = None,
        ef: int = 100,
        with_payload: bool = True,
        with_vector: bool = False,
        score_threshold: Optional[float] = None
    ) -> List[Match]:
        """
        Search for similar vectors.

        Args:
            collection_name: Collection to search
            vector: Query vector
            query_name: Specific named_vector matrix space
            limit: Maximum number of results
            filter: Filter conditions
            ef: HNSW ef_search parameter
            with_payload: Include payloads in results
            with_vector: Include vectors in results
            score_threshold: Minimum score threshold

        Returns:
            List of Match objects with id, score, payload, vector
        """
        payload: Dict[str, Any] = {
            "limit": limit,
            "ef": ef,
            "with_payload": with_payload,
            "with_vector": with_vector
        }
        if vector:
            payload["vector"] = vector
        if query_name:
            payload["query_name"] = query_name
        if filter:
            payload["filter"] = filter
        if score_threshold is not None:
            payload["score_threshold"] = score_threshold

        res = self._post(f"/collections/{collection_name}/search", json=payload)
        return [Match(**hit) for hit in res.get("result", [])]

    def search_batch(
        self,
        collection_name: str,
        vectors: List[List[float]],
        limit: int = 10,
        filter: Optional[Dict[str, Any]] = None,
        with_payload: bool = True
    ) -> List[List[Match]]:
        """
        Search for multiple vectors in batch.

        Args:
            collection_name: Collection to search
            vectors: List of query vectors
            limit: Maximum results per query
            filter: Filter conditions
            with_payload: Include payloads

        Returns:
            List of result lists, one per query vector
        """
        searches = []
        for vec in vectors:
            search: Dict[str, Any] = {
                "vector": vec,
                "limit": limit,
                "with_payload": with_payload
            }
            if filter:
                search["filter"] = filter
            searches.append(search)

        payload = {"searches": searches}
        res = self._post(f"/collections/{collection_name}/search/batch", json=payload)

        results = []
        for batch in res.get("results", []):
            results.append([Match(**hit) for hit in batch.get("result", [])])
        return results

    def hybrid_search(
        self,
        collection_name: str,
        dense_vector: Optional[List[float]] = None,
        sparse_query: Optional[Union[Dict[str, Any], SparseVector]] = None,
        limit: int = 10,
        filter: Optional[Dict[str, Any]] = None,
        fusion_method: str = "rrf",
        fusion_k: int = 60,
        with_payload: bool = True
    ) -> List[Match]:
        """
        Perform hybrid search combining dense and sparse vectors.

        Args:
            collection_name: Collection to search
            dense_vector: Dense embedding vector
            sparse_query: Sparse representation with indices and values
            limit: Maximum results
            filter: Filter conditions
            fusion_method: Fusion method (rrf, linear)
            fusion_k: RRF k parameter
            with_payload: Include payloads

        Returns:
            List of Match objects
        """
        payload: Dict[str, Any] = {
            "limit": limit,
            "with_payload": with_payload,
            "fusion": {
                "method": fusion_method,
                "k": fusion_k
            }
        }
        if dense_vector is not None:
            payload["dense_vector"] = dense_vector
        if sparse_query is not None:
            if isinstance(sparse_query, SparseVector):
                payload["sparse_query"] = sparse_query.model_dump()
            else:
                payload["sparse_query"] = sparse_query
        if filter:
            payload["filter"] = filter

        res = self._post(f"/collections/{collection_name}/search/hybrid", json=payload)
        return [Match(**hit) for hit in res.get("results", [])]

    def discover(self, collection_name: str, params: DiscoverParams) -> List[Match]:
        """
        Discover similar points using context.

        Args:
            collection_name: Collection to search
            params: DiscoverParams with target and context

        Returns:
            List of Match objects
        """
        res = self._post(f"/collections/{collection_name}/discover", json=params.model_dump(exclude_none=True))
        return [Match(**hit) for hit in res.get("points", [])]

    def scroll(
        self,
        collection_name: str,
        limit: int = 100,
        offset: Optional[int] = None,
        filter: Optional[Dict[str, Any]] = None,
        with_payload: bool = True,
        with_vector: bool = False
    ) -> dict:
        """
        Scroll through points in a collection.

        Args:
            collection_name: Collection to scroll
            limit: Number of points per page
            offset: Starting offset
            filter: Filter conditions
            with_payload: Include payloads
            with_vector: Include vectors

        Returns:
            Dict with 'points' and 'next_offset'
        """
        payload: Dict[str, Any] = {
            "limit": limit,
            "with_payload": with_payload,
            "with_vector": with_vector
        }
        if offset is not None:
            payload["offset"] = offset
        if filter:
            payload["filter"] = filter

        return self._post(f"/collections/{collection_name}/points/scroll", json=payload)

    # Utility methods

    def health(self) -> dict:
        """Check server health."""
        return self._get("/health")

    def info(self) -> dict:
        """Get server info."""
        return self._get("/info")

    def close(self):
        """Close the client session."""
        self.session.close()

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()
