"""
LimyeDB LangChain Integration

This module provides a LangChain VectorStore implementation for LimyeDB.

Example:
    >>> from langchain_openai import OpenAIEmbeddings
    >>> from limyedb_langchain import LimyeDB
    >>>
    >>> embeddings = OpenAIEmbeddings()
    >>> vectorstore = LimyeDB(
    ...     url="http://localhost:8080",
    ...     api_key="your-api-key",
    ...     collection_name="documents",
    ...     embedding=embeddings
    ... )
    >>>
    >>> # Add documents
    >>> vectorstore.add_texts(["Hello world", "LimyeDB is fast"])
    >>>
    >>> # Search
    >>> results = vectorstore.similarity_search("hello", k=5)
"""

from typing import Any, Dict, Iterable, List, Optional, Tuple, Type
import uuid

try:
    from langchain_core.documents import Document
    from langchain_core.embeddings import Embeddings
    from langchain_core.vectorstores import VectorStore
except ImportError:
    from langchain.schema import Document
    from langchain.embeddings.base import Embeddings
    from langchain.vectorstores.base import VectorStore

import requests


class LimyeDB(VectorStore):
    """
    LimyeDB VectorStore implementation for LangChain.

    Args:
        url: LimyeDB server URL
        api_key: API key for authentication
        collection_name: Name of the collection
        embedding: Embedding function to use
        content_payload_key: Key to store document content in payload
        metadata_payload_key: Key to store metadata in payload
    """

    def __init__(
        self,
        url: str = "http://localhost:8080",
        api_key: Optional[str] = None,
        collection_name: str = "langchain",
        embedding: Optional[Embeddings] = None,
        content_payload_key: str = "page_content",
        metadata_payload_key: str = "metadata",
        distance_strategy: str = "cosine",
        **kwargs: Any,
    ):
        self.url = url.rstrip("/")
        self.api_key = api_key
        self.collection_name = collection_name
        self._embedding = embedding
        self.content_payload_key = content_payload_key
        self.metadata_payload_key = metadata_payload_key
        self.distance_strategy = distance_strategy

        self.session = requests.Session()
        self.session.headers["Content-Type"] = "application/json"
        if api_key:
            self.session.headers["Authorization"] = f"Bearer {api_key}"

    @property
    def embeddings(self) -> Optional[Embeddings]:
        return self._embedding

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

    def add_texts(
        self,
        texts: Iterable[str],
        metadatas: Optional[List[Dict]] = None,
        ids: Optional[List[str]] = None,
        **kwargs: Any,
    ) -> List[str]:
        """
        Add texts to the vector store.

        Args:
            texts: Iterable of texts to add
            metadatas: Optional list of metadata dicts
            ids: Optional list of IDs

        Returns:
            List of IDs of added texts
        """
        texts_list = list(texts)

        if self._embedding is None:
            raise ValueError("Embedding function is required")

        embeddings = self._embedding.embed_documents(texts_list)

        self._ensure_collection(len(embeddings[0]))

        if ids is None:
            ids = [str(uuid.uuid4()) for _ in texts_list]

        if metadatas is None:
            metadatas = [{} for _ in texts_list]

        points = []
        for i, (text, embedding, metadata, id_) in enumerate(
            zip(texts_list, embeddings, metadatas, ids)
        ):
            payload = {
                self.content_payload_key: text,
                self.metadata_payload_key: metadata,
            }
            payload.update(metadata)

            points.append({
                "id": id_,
                "vector": embedding,
                "payload": payload
            })

        self._request(
            "PUT",
            f"/collections/{self.collection_name}/points",
            {"points": points}
        )

        return ids

    def similarity_search(
        self,
        query: str,
        k: int = 4,
        filter: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> List[Document]:
        """
        Search for similar documents.

        Args:
            query: Query text
            k: Number of results to return
            filter: Optional filter conditions

        Returns:
            List of Documents
        """
        docs_and_scores = self.similarity_search_with_score(
            query=query,
            k=k,
            filter=filter,
            **kwargs
        )
        return [doc for doc, _ in docs_and_scores]

    def similarity_search_with_score(
        self,
        query: str,
        k: int = 4,
        filter: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> List[Tuple[Document, float]]:
        """
        Search for similar documents with scores.

        Args:
            query: Query text
            k: Number of results to return
            filter: Optional filter conditions

        Returns:
            List of (Document, score) tuples
        """
        if self._embedding is None:
            raise ValueError("Embedding function is required")

        embedding = self._embedding.embed_query(query)
        return self.similarity_search_by_vector_with_score(
            embedding=embedding,
            k=k,
            filter=filter,
            **kwargs
        )

    def similarity_search_by_vector(
        self,
        embedding: List[float],
        k: int = 4,
        filter: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> List[Document]:
        """
        Search by vector.

        Args:
            embedding: Query vector
            k: Number of results
            filter: Optional filter

        Returns:
            List of Documents
        """
        docs_and_scores = self.similarity_search_by_vector_with_score(
            embedding=embedding,
            k=k,
            filter=filter,
            **kwargs
        )
        return [doc for doc, _ in docs_and_scores]

    def similarity_search_by_vector_with_score(
        self,
        embedding: List[float],
        k: int = 4,
        filter: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> List[Tuple[Document, float]]:
        """
        Search by vector with scores.

        Args:
            embedding: Query vector
            k: Number of results
            filter: Optional filter

        Returns:
            List of (Document, score) tuples
        """
        payload = {
            "vector": embedding,
            "limit": k,
            "with_payload": True
        }

        if filter:
            payload["filter"] = filter

        result = self._request(
            "POST",
            f"/collections/{self.collection_name}/search",
            payload
        )

        docs_and_scores = []
        for hit in result.get("result", []):
            payload = hit.get("payload", {})
            content = payload.get(self.content_payload_key, "")
            metadata = payload.get(self.metadata_payload_key, {})

            for key in payload:
                if key not in (self.content_payload_key, self.metadata_payload_key):
                    metadata[key] = payload[key]

            doc = Document(page_content=content, metadata=metadata)
            docs_and_scores.append((doc, hit.get("score", 0.0)))

        return docs_and_scores

    def max_marginal_relevance_search(
        self,
        query: str,
        k: int = 4,
        fetch_k: int = 20,
        lambda_mult: float = 0.5,
        filter: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> List[Document]:
        """
        Maximum Marginal Relevance search.

        Args:
            query: Query text
            k: Number of results
            fetch_k: Number of candidates to fetch
            lambda_mult: MMR lambda parameter
            filter: Optional filter

        Returns:
            List of Documents
        """
        if self._embedding is None:
            raise ValueError("Embedding function is required")

        embedding = self._embedding.embed_query(query)

        docs_and_scores = self.similarity_search_by_vector_with_score(
            embedding=embedding,
            k=fetch_k,
            filter=filter,
            **kwargs
        )

        if len(docs_and_scores) <= k:
            return [doc for doc, _ in docs_and_scores]

        selected = []
        remaining = list(range(len(docs_and_scores)))

        while len(selected) < k and remaining:
            best_idx = remaining[0]
            best_score = -float("inf")

            for idx in remaining:
                doc, score = docs_and_scores[idx]
                mmr_score = lambda_mult * score

                if selected:
                    max_sim = max(
                        self._cosine_similarity(
                            docs_and_scores[idx][0].page_content,
                            docs_and_scores[s][0].page_content
                        )
                        for s in selected
                    )
                    mmr_score -= (1 - lambda_mult) * max_sim

                if mmr_score > best_score:
                    best_score = mmr_score
                    best_idx = idx

            selected.append(best_idx)
            remaining.remove(best_idx)

        return [docs_and_scores[i][0] for i in selected]

    def _cosine_similarity(self, text1: str, text2: str) -> float:
        """Simple cosine similarity based on character overlap."""
        set1 = set(text1.lower().split())
        set2 = set(text2.lower().split())
        intersection = len(set1 & set2)
        union = len(set1 | set2)
        return intersection / union if union > 0 else 0.0

    def delete(
        self,
        ids: Optional[List[str]] = None,
        **kwargs: Any
    ) -> Optional[bool]:
        """
        Delete documents by IDs.

        Args:
            ids: List of document IDs to delete

        Returns:
            True if successful
        """
        if ids is None:
            return False

        self._request(
            "POST",
            f"/collections/{self.collection_name}/points/delete",
            {"ids": ids}
        )
        return True

    @classmethod
    def from_texts(
        cls: Type["LimyeDB"],
        texts: List[str],
        embedding: Embeddings,
        metadatas: Optional[List[Dict]] = None,
        ids: Optional[List[str]] = None,
        url: str = "http://localhost:8080",
        api_key: Optional[str] = None,
        collection_name: str = "langchain",
        **kwargs: Any,
    ) -> "LimyeDB":
        """
        Create a LimyeDB instance from texts.

        Args:
            texts: List of texts
            embedding: Embedding function
            metadatas: Optional metadata
            ids: Optional IDs
            url: LimyeDB URL
            api_key: API key
            collection_name: Collection name

        Returns:
            LimyeDB instance
        """
        instance = cls(
            url=url,
            api_key=api_key,
            collection_name=collection_name,
            embedding=embedding,
            **kwargs
        )
        instance.add_texts(texts=texts, metadatas=metadatas, ids=ids)
        return instance

    @classmethod
    def from_documents(
        cls: Type["LimyeDB"],
        documents: List[Document],
        embedding: Embeddings,
        url: str = "http://localhost:8080",
        api_key: Optional[str] = None,
        collection_name: str = "langchain",
        **kwargs: Any,
    ) -> "LimyeDB":
        """
        Create a LimyeDB instance from documents.

        Args:
            documents: List of Documents
            embedding: Embedding function
            url: LimyeDB URL
            api_key: API key
            collection_name: Collection name

        Returns:
            LimyeDB instance
        """
        texts = [doc.page_content for doc in documents]
        metadatas = [doc.metadata for doc in documents]
        return cls.from_texts(
            texts=texts,
            embedding=embedding,
            metadatas=metadatas,
            url=url,
            api_key=api_key,
            collection_name=collection_name,
            **kwargs
        )
