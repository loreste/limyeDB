# Building RAG Applications with LimyeDB

This tutorial guides you through building a Retrieval-Augmented Generation (RAG) application using LimyeDB as the vector store.

## Table of Contents

1. [What is RAG?](#what-is-rag)
2. [Architecture Overview](#architecture-overview)
3. [Setup](#setup)
4. [Document Processing](#document-processing)
5. [Indexing Documents](#indexing-documents)
6. [Retrieval](#retrieval)
7. [Generation](#generation)
8. [Complete Application](#complete-application)
9. [Production Considerations](#production-considerations)

---

## What is RAG?

Retrieval-Augmented Generation (RAG) enhances Large Language Models (LLMs) by:

1. **Retrieving** relevant context from a knowledge base
2. **Augmenting** the prompt with this context
3. **Generating** responses grounded in your data

### Benefits

- **Reduced Hallucinations**: Responses are grounded in actual documents
- **Up-to-date Information**: Knowledge base can be updated without retraining
- **Source Attribution**: Cite sources for generated answers
- **Domain Expertise**: Inject specialized knowledge into general-purpose LLMs

---

## Architecture Overview

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   User      │     │  LimyeDB    │     │    LLM      │
│   Query     │────▶│  Vector DB  │────▶│  (GPT/etc)  │
└─────────────┘     └─────────────┘     └─────────────┘
      │                   │                    │
      │                   │                    │
      │   1. Embed query  │                    │
      │   2. Search       │                    │
      │   3. Get context ─┘                    │
      │   4. Augment prompt ──────────────────▶│
      │   5. Generate response ◀───────────────│
      └────────────────────────────────────────┘
```

---

## Setup

### Install Dependencies

```bash
pip install limyedb sentence-transformers openai tiktoken
```

### Initialize Clients

```python
from limyedb import LimyeDBClient
from sentence_transformers import SentenceTransformer
from openai import OpenAI
import tiktoken

# Vector database
limye_client = LimyeDBClient(host="http://localhost:8080")

# Embedding model
embed_model = SentenceTransformer("all-MiniLM-L6-v2")  # 384 dims

# LLM
openai_client = OpenAI(api_key="your-api-key")

# Tokenizer for chunk sizing
tokenizer = tiktoken.encoding_for_model("gpt-4")
```

---

## Document Processing

### Text Chunking

Split documents into chunks that fit the embedding model's context window.

```python
def chunk_text(text: str, chunk_size: int = 500, overlap: int = 50) -> list[str]:
    """Split text into overlapping chunks by token count."""
    tokens = tokenizer.encode(text)
    chunks = []

    start = 0
    while start < len(tokens):
        end = start + chunk_size
        chunk_tokens = tokens[start:end]
        chunk_text = tokenizer.decode(chunk_tokens)
        chunks.append(chunk_text)
        start = end - overlap

    return chunks


def chunk_by_sentences(text: str, max_tokens: int = 500) -> list[str]:
    """Split text by sentences, respecting token limits."""
    import re

    sentences = re.split(r'(?<=[.!?])\s+', text)
    chunks = []
    current_chunk = []
    current_tokens = 0

    for sentence in sentences:
        sentence_tokens = len(tokenizer.encode(sentence))

        if current_tokens + sentence_tokens > max_tokens:
            if current_chunk:
                chunks.append(' '.join(current_chunk))
            current_chunk = [sentence]
            current_tokens = sentence_tokens
        else:
            current_chunk.append(sentence)
            current_tokens += sentence_tokens

    if current_chunk:
        chunks.append(' '.join(current_chunk))

    return chunks
```

### Document Loading

```python
import os
from pathlib import Path

def load_documents(directory: str) -> list[dict]:
    """Load documents from a directory."""
    documents = []

    for filepath in Path(directory).glob("**/*.txt"):
        with open(filepath, 'r') as f:
            content = f.read()

        documents.append({
            "id": str(filepath),
            "content": content,
            "metadata": {
                "filename": filepath.name,
                "path": str(filepath),
                "size": len(content)
            }
        })

    return documents


def load_pdf(filepath: str) -> str:
    """Load text from a PDF file."""
    import pypdf

    reader = pypdf.PdfReader(filepath)
    text = ""
    for page in reader.pages:
        text += page.extract_text() + "\n"

    return text
```

---

## Indexing Documents

### Create Collection

```python
# Create collection for document chunks
limye_client.create_collection(
    name="knowledge_base",
    dimension=384,  # all-MiniLM-L6-v2 dimension
    metric="cosine",
    hnsw={
        "m": 16,
        "ef_construction": 200
    }
)

# Create payload index for filtering
limye_client.create_payload_index(
    collection_name="knowledge_base",
    field_name="source",
    field_schema="keyword"
)
```

### Index Documents

```python
from limyedb import Point
from uuid import uuid4

def index_documents(documents: list[dict], collection_name: str = "knowledge_base"):
    """Process and index documents."""
    all_points = []

    for doc in documents:
        # Chunk the document
        chunks = chunk_by_sentences(doc["content"], max_tokens=500)

        for i, chunk in enumerate(chunks):
            # Generate embedding
            embedding = embed_model.encode(chunk).tolist()

            # Create point
            point = Point.create(
                id=f"{doc['id']}_chunk_{i}",
                vector=embedding,
                payload={
                    "text": chunk,
                    "source": doc["metadata"]["filename"],
                    "chunk_index": i,
                    "total_chunks": len(chunks),
                    **doc["metadata"]
                }
            )
            all_points.append(point)

    # Batch insert
    batch_size = 100
    for i in range(0, len(all_points), batch_size):
        batch = all_points[i:i+batch_size]
        limye_client.upsert(collection_name, batch)
        print(f"Indexed {min(i+batch_size, len(all_points))}/{len(all_points)} chunks")

    print(f"Indexing complete: {len(all_points)} chunks from {len(documents)} documents")


# Usage
documents = load_documents("./docs")
index_documents(documents)
```

---

## Retrieval

### Basic Retrieval

```python
def retrieve(query: str, top_k: int = 5, filter: dict = None) -> list[dict]:
    """Retrieve relevant chunks for a query."""
    # Embed query
    query_vector = embed_model.encode(query).tolist()

    # Search
    results = limye_client.search(
        collection_name="knowledge_base",
        query_vector=query_vector,
        limit=top_k,
        filter=filter,
        with_payload=True
    )

    return [
        {
            "text": r.payload["text"],
            "source": r.payload["source"],
            "score": r.score
        }
        for r in results
    ]
```

### Retrieval with Reranking

For better results, rerank retrieved chunks:

```python
from sentence_transformers import CrossEncoder

reranker = CrossEncoder("cross-encoder/ms-marco-MiniLM-L-6-v2")

def retrieve_and_rerank(query: str, top_k: int = 5, initial_k: int = 20) -> list[dict]:
    """Retrieve and rerank chunks."""
    # Initial retrieval (over-fetch)
    query_vector = embed_model.encode(query).tolist()

    results = limye_client.search(
        collection_name="knowledge_base",
        query_vector=query_vector,
        limit=initial_k,
        with_payload=True
    )

    # Rerank
    pairs = [(query, r.payload["text"]) for r in results]
    scores = reranker.predict(pairs)

    # Sort by rerank score
    reranked = sorted(
        zip(results, scores),
        key=lambda x: x[1],
        reverse=True
    )[:top_k]

    return [
        {
            "text": r.payload["text"],
            "source": r.payload["source"],
            "score": float(score),
            "vector_score": r.score
        }
        for r, score in reranked
    ]
```

### Hybrid Retrieval

Combine dense and sparse search for better results:

```python
def hybrid_retrieve(query: str, top_k: int = 5, alpha: float = 0.7) -> list[dict]:
    """Hybrid retrieval combining dense and sparse search."""
    # Dense search
    query_vector = embed_model.encode(query).tolist()

    results = limye_client.hybrid_search(
        collection_name="knowledge_base",
        query_vector=query_vector,
        query_text=query,  # For BM25/sparse
        limit=top_k,
        alpha=alpha,  # Weight towards dense (0=sparse, 1=dense)
        with_payload=True
    )

    return [
        {
            "text": r.payload["text"],
            "source": r.payload["source"],
            "score": r.score
        }
        for r in results
    ]
```

---

## Generation

### Prompt Template

```python
SYSTEM_PROMPT = """You are a helpful assistant that answers questions based on the provided context.

Guidelines:
- Answer based ONLY on the provided context
- If the context doesn't contain enough information, say so
- Cite sources when possible using [Source: filename]
- Be concise and accurate
"""

def create_prompt(query: str, context: list[dict]) -> str:
    """Create a prompt with retrieved context."""
    context_text = "\n\n".join([
        f"[Source: {c['source']}]\n{c['text']}"
        for c in context
    ])

    return f"""Context:
{context_text}

Question: {query}

Answer:"""
```

### Generate Response

```python
def generate_response(query: str, context: list[dict]) -> str:
    """Generate a response using the LLM."""
    prompt = create_prompt(query, context)

    response = openai_client.chat.completions.create(
        model="gpt-4",
        messages=[
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": prompt}
        ],
        temperature=0.1,
        max_tokens=500
    )

    return response.choices[0].message.content
```

### Streaming Response

```python
def generate_response_stream(query: str, context: list[dict]):
    """Generate a streaming response."""
    prompt = create_prompt(query, context)

    stream = openai_client.chat.completions.create(
        model="gpt-4",
        messages=[
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": prompt}
        ],
        temperature=0.1,
        max_tokens=500,
        stream=True
    )

    for chunk in stream:
        if chunk.choices[0].delta.content:
            yield chunk.choices[0].delta.content
```

---

## Complete Application

### RAG Class

```python
class RAGApplication:
    def __init__(
        self,
        limye_host: str = "http://localhost:8080",
        collection_name: str = "knowledge_base",
        embed_model_name: str = "all-MiniLM-L6-v2",
        llm_model: str = "gpt-4"
    ):
        self.collection_name = collection_name
        self.llm_model = llm_model

        # Initialize clients
        self.limye = LimyeDBClient(host=limye_host)
        self.embed_model = SentenceTransformer(embed_model_name)
        self.openai = OpenAI()
        self.tokenizer = tiktoken.encoding_for_model(llm_model)

    def setup_collection(self, dimension: int = 384):
        """Create the collection if it doesn't exist."""
        if not self.limye.collection_exists(self.collection_name):
            self.limye.create_collection(
                name=self.collection_name,
                dimension=dimension,
                metric="cosine"
            )

    def index_document(self, document: str, metadata: dict = None):
        """Index a single document."""
        chunks = self._chunk_text(document)
        points = []

        for i, chunk in enumerate(chunks):
            embedding = self.embed_model.encode(chunk).tolist()
            points.append(Point.create(
                id=f"{uuid4()}_chunk_{i}",
                vector=embedding,
                payload={
                    "text": chunk,
                    "chunk_index": i,
                    **(metadata or {})
                }
            ))

        self.limye.upsert(self.collection_name, points)
        return len(points)

    def query(
        self,
        question: str,
        top_k: int = 5,
        filter: dict = None,
        stream: bool = False
    ):
        """Answer a question using RAG."""
        # Retrieve
        context = self._retrieve(question, top_k, filter)

        # Generate
        if stream:
            return self._generate_stream(question, context)
        else:
            return self._generate(question, context), context

    def _chunk_text(self, text: str, max_tokens: int = 500) -> list[str]:
        """Chunk text by token count."""
        tokens = self.tokenizer.encode(text)
        chunks = []

        for i in range(0, len(tokens), max_tokens - 50):
            chunk_tokens = tokens[i:i + max_tokens]
            chunks.append(self.tokenizer.decode(chunk_tokens))

        return chunks

    def _retrieve(self, query: str, top_k: int, filter: dict = None) -> list[dict]:
        """Retrieve relevant context."""
        query_vector = self.embed_model.encode(query).tolist()

        results = self.limye.search(
            collection_name=self.collection_name,
            query_vector=query_vector,
            limit=top_k,
            filter=filter,
            with_payload=True
        )

        return [
            {"text": r.payload["text"], "score": r.score, **r.payload}
            for r in results
        ]

    def _generate(self, question: str, context: list[dict]) -> str:
        """Generate answer."""
        prompt = self._create_prompt(question, context)

        response = self.openai.chat.completions.create(
            model=self.llm_model,
            messages=[
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": prompt}
            ],
            temperature=0.1
        )

        return response.choices[0].message.content

    def _generate_stream(self, question: str, context: list[dict]):
        """Generate streaming answer."""
        prompt = self._create_prompt(question, context)

        stream = self.openai.chat.completions.create(
            model=self.llm_model,
            messages=[
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": prompt}
            ],
            temperature=0.1,
            stream=True
        )

        for chunk in stream:
            if chunk.choices[0].delta.content:
                yield chunk.choices[0].delta.content

    def _create_prompt(self, question: str, context: list[dict]) -> str:
        """Create augmented prompt."""
        context_text = "\n\n".join([
            f"[Score: {c['score']:.3f}]\n{c['text']}"
            for c in context
        ])

        return f"Context:\n{context_text}\n\nQuestion: {question}\n\nAnswer:"
```

### Usage Example

```python
# Initialize
rag = RAGApplication()
rag.setup_collection()

# Index documents
documents = [
    "LimyeDB is a high-performance vector database for AI applications.",
    "HNSW is the default index algorithm providing fast approximate nearest neighbor search.",
    "Quantization reduces memory usage by compressing vector representations."
]

for doc in documents:
    rag.index_document(doc, metadata={"source": "manual"})

# Query
answer, sources = rag.query("What is LimyeDB?")
print(f"Answer: {answer}")
print(f"\nSources:")
for s in sources:
    print(f"  - {s['text'][:100]}... (score: {s['score']:.3f})")
```

---

## Production Considerations

### 1. Embedding Model Selection

| Model | Dimensions | Speed | Quality |
|-------|------------|-------|---------|
| all-MiniLM-L6-v2 | 384 | Fast | Good |
| all-mpnet-base-v2 | 768 | Medium | Better |
| text-embedding-3-small | 1536 | API | Best |

### 2. Chunk Size Optimization

```python
# Test different chunk sizes
chunk_sizes = [256, 512, 1024]
for size in chunk_sizes:
    recall = evaluate_retrieval(test_queries, chunk_size=size)
    print(f"Chunk size {size}: Recall@5 = {recall:.3f}")
```

### 3. Caching

```python
from functools import lru_cache
import hashlib

@lru_cache(maxsize=1000)
def cached_embed(text: str) -> tuple:
    """Cache embeddings for repeated queries."""
    return tuple(embed_model.encode(text).tolist())

def retrieve_cached(query: str, top_k: int = 5):
    query_vector = list(cached_embed(query))
    return limye_client.search(...)
```

### 4. Monitoring

```python
import time
import logging

logger = logging.getLogger(__name__)

def monitored_query(question: str, **kwargs):
    start = time.time()

    # Retrieve
    retrieve_start = time.time()
    context = retrieve(question, **kwargs)
    retrieve_time = time.time() - retrieve_start

    # Generate
    generate_start = time.time()
    answer = generate_response(question, context)
    generate_time = time.time() - generate_start

    total_time = time.time() - start

    logger.info(f"RAG query completed", extra={
        "question_length": len(question),
        "retrieve_time_ms": retrieve_time * 1000,
        "generate_time_ms": generate_time * 1000,
        "total_time_ms": total_time * 1000,
        "context_chunks": len(context)
    })

    return answer, context
```

### 5. Error Handling

```python
from tenacity import retry, stop_after_attempt, wait_exponential

@retry(
    stop=stop_after_attempt(3),
    wait=wait_exponential(multiplier=1, min=1, max=10)
)
def robust_query(question: str, **kwargs):
    """Query with automatic retry."""
    try:
        return rag.query(question, **kwargs)
    except Exception as e:
        logger.error(f"RAG query failed: {e}")
        raise
```

### 6. Security

```python
def sanitize_query(query: str) -> str:
    """Sanitize user input."""
    # Remove potential injection attempts
    query = query.replace("{", "").replace("}", "")
    # Limit length
    query = query[:1000]
    return query.strip()

def secure_query(question: str, user_id: str, **kwargs):
    """Query with access control."""
    sanitized = sanitize_query(question)

    # Add user-specific filter
    filter = {
        "must": [
            {"key": "access_level", "match": {"any": ["public", user_id]}}
        ]
    }

    return rag.query(sanitized, filter=filter, **kwargs)
```

---

## Next Steps

- [Hybrid Search Deep Dive](hybrid_search_deep_dive.md) - Improve retrieval quality
- [Scaling to Millions](scaling_to_millions.md) - Handle large document collections
- [Performance Tuning](../performance_tuning.md) - Optimize for your workload

