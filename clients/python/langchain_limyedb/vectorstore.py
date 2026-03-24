import uuid
from typing import Any, Iterable, List, Optional, Tuple, Dict

from langchain_core.documents import Document
from langchain_core.embeddings import Embeddings
from langchain_core.vectorstores import VectorStore

from limyedb.client import LimyeDBClient
from limyedb.models import Point, CollectionConfig

class LimyeDBContext(VectorStore):
    """LangChain native wrapper bounding LimyeDB into standard RAG retrieval patterns."""

    def __init__(
        self,
        client: LimyeDBClient,
        collection_name: str,
        embedding: Embeddings,
    ):
        self.client = client
        self.collection_name = collection_name
        self.embedding = embedding

    def add_texts(
        self,
        texts: Iterable[str],
        metadatas: Optional[List[dict]] = None,
        ids: Optional[List[str]] = None,
        **kwargs: Any,
    ) -> List[str]:
        """Run texts through embeddings and upload into LimyeDB."""
        texts = list(texts)
        if not texts:
            return []
            
        embeddings = self.embedding.embed_documents(texts)
        if ids is None:
            ids = [str(uuid.uuid4()) for _ in texts]
        if metadatas is None:
            metadatas = [{} for _ in texts]
            
        points = []
        for text, embed, metadata, doc_id in zip(texts, embeddings, metadatas, ids):
            # We inject the string content straight into payload['page_content']
            payload = metadata.copy()
            payload["page_content"] = text
            points.append(Point(id=doc_id, vector=embed, payload=payload))
            
        # Push to LimyeDB
        self.client.upsert(self.collection_name, points)
        return ids

    def similarity_search(
        self, 
        query: str, 
        k: int = 4, 
        filter: Optional[Dict[str, Any]] = None,
        **kwargs: Any
    ) -> List[Document]:
        """Return docs most similar to query."""
        embedding = self.embedding.embed_query(query)
        matches = self.client.search(
            collection_name=self.collection_name,
            vector=embedding,
            limit=k,
            filter=filter
        )
        
        docs = []
        for match in matches:
            payload = match.payload or {}
            content = payload.pop("page_content", "")
            docs.append(Document(page_content=content, metadata=payload))
            
        return docs

    def similarity_search_with_score(
        self,
        query: str,
        k: int = 4,
        filter: Optional[Dict[str, Any]] = None,
        **kwargs: Any
    ) -> List[Tuple[Document, float]]:
        """Return docs and relevance scores."""
        embedding = self.embedding.embed_query(query)
        matches = self.client.search(
            collection_name=self.collection_name,
            vector=embedding,
            limit=k,
            filter=filter
        )
        
        results = []
        for match in matches:
            payload = match.payload or {}
            content = payload.pop("page_content", "")
            doc = Document(page_content=content, metadata=payload)
            results.append((doc, match.score))
            
        return results

    @classmethod
    def from_texts(
        cls,
        texts: List[str],
        embedding: Embeddings,
        metadatas: Optional[List[dict]] = None,
        client: Optional[LimyeDBClient] = None,
        collection_name: str = "langchain_default",
        **kwargs: Any,
    ) -> "LimyeDBContext":
        """Factory pattern bounding raw texts straight into an initialized pipeline."""
        if client is None:
            client = LimyeDBClient()
            
        # Validate/Create Collection
        try:
            client.get_collection(collection_name)
        except Exception:
            # Grab vector dimension dynamically from first text
            dim = len(embedding.embed_query(texts[0]))
            client.create_collection(CollectionConfig(
                name=collection_name, 
                dimension=dim, 
                metric="cosine", 
                on_disk=True
            ))
            
        instance = cls(client, collection_name, embedding)
        instance.add_texts(texts, metadatas=metadatas, **kwargs)
        return instance
