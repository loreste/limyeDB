# Hybrid Search Deep Dive

This tutorial explores hybrid search in LimyeDB, combining dense vector search with sparse keyword search for improved retrieval quality.

## Table of Contents

1. [Understanding Hybrid Search](#understanding-hybrid-search)
2. [Dense vs. Sparse Representations](#dense-vs-sparse-representations)
3. [Setting Up Hybrid Search](#setting-up-hybrid-search)
4. [Fusion Strategies](#fusion-strategies)
5. [Tuning Alpha Parameter](#tuning-alpha-parameter)
6. [Advanced Techniques](#advanced-techniques)
7. [Benchmarking](#benchmarking)
8. [Best Practices](#best-practices)

---

## Understanding Hybrid Search

Hybrid search combines two complementary retrieval approaches:

### Dense Search (Semantic)
- Uses neural network embeddings
- Captures semantic meaning
- Good for conceptual similarity
- "King is to Queen as Man is to Woman"

### Sparse Search (Lexical)
- Uses keyword matching (BM25, TF-IDF)
- Exact term matching
- Good for specific terms, names, codes
- "Find documents mentioning 'TCP/IP'"

### Why Combine Them?

| Query Type | Dense Alone | Sparse Alone | Hybrid |
|------------|-------------|--------------|--------|
| Semantic ("machine learning techniques") | ✅ Excellent | ❌ Limited | ✅ Excellent |
| Exact terms ("error code E-1234") | ❌ Poor | ✅ Excellent | ✅ Excellent |
| Mixed ("Python pandas tutorial") | ⚠️ Good | ⚠️ Good | ✅ Best |

---

## Dense vs. Sparse Representations

### Dense Vectors

Dense vectors have values in every dimension:

```python
# 384-dimensional dense vector (all values non-zero)
dense_vector = [0.023, -0.156, 0.089, 0.234, ..., 0.012]  # 384 floats
```

**Characteristics:**
- Fixed dimension (e.g., 384, 768, 1536)
- Learned from neural networks
- Captures semantic relationships
- Memory: `4 bytes × dimension`

### Sparse Vectors

Sparse vectors have mostly zero values:

```python
# Sparse vector representation
sparse_vector = {
    "indices": [42, 156, 2891, 5432],  # Non-zero positions
    "values": [0.8, 0.3, 0.5, 0.2]      # Corresponding values
}
```

**Characteristics:**
- Variable dimension (vocabulary size, e.g., 30K tokens)
- Based on term frequency statistics
- Captures lexical importance
- Memory: Only non-zero values stored

---

## Setting Up Hybrid Search

### Create Collection with Both Vector Types

```python
from limyedb import LimyeDBClient

client = LimyeDBClient(host="http://localhost:8080")

# Create collection with named vectors
client.create_collection(
    name="documents",
    vectors={
        "dense": {
            "dimension": 384,
            "metric": "cosine"
        },
        "sparse": {
            "dimension": 30000,  # Vocabulary size
            "metric": "dot_product",
            "sparse": True
        }
    }
)
```

### Generate Dense Embeddings

```python
from sentence_transformers import SentenceTransformer

dense_model = SentenceTransformer("all-MiniLM-L6-v2")

def get_dense_embedding(text: str) -> list[float]:
    return dense_model.encode(text).tolist()
```

### Generate Sparse Embeddings

```python
from transformers import AutoTokenizer, AutoModel
import torch

# Option 1: SPLADE model
class SPLADEEncoder:
    def __init__(self, model_name: str = "naver/splade-cocondenser-ensembledistil"):
        self.tokenizer = AutoTokenizer.from_pretrained(model_name)
        self.model = AutoModel.from_pretrained(model_name)
        self.model.eval()

    def encode(self, text: str) -> dict:
        tokens = self.tokenizer(text, return_tensors="pt", truncation=True, max_length=512)

        with torch.no_grad():
            output = self.model(**tokens)
            logits = output.logits

        # Get sparse representation
        weights = torch.max(torch.log1p(torch.relu(logits)), dim=1)[0].squeeze()

        # Extract non-zero indices and values
        non_zero = weights.nonzero().squeeze()
        indices = non_zero.tolist()
        values = weights[non_zero].tolist()

        return {"indices": indices, "values": values}

splade = SPLADEEncoder()


# Option 2: Simple BM25-style
from collections import Counter
import math

class BM25Encoder:
    def __init__(self, vocab_size: int = 30000):
        self.vocab_size = vocab_size

    def encode(self, text: str) -> dict:
        # Simple tokenization
        tokens = text.lower().split()
        term_freq = Counter(tokens)

        indices = []
        values = []

        for term, freq in term_freq.items():
            # Hash to vocabulary index
            idx = hash(term) % self.vocab_size
            # TF-IDF-like weight
            weight = (1 + math.log(freq)) if freq > 0 else 0
            indices.append(idx)
            values.append(weight)

        return {"indices": indices, "values": values}

bm25 = BM25Encoder()
```

### Index Documents

```python
from limyedb import Point, SparseVector

def index_document(doc_id: str, text: str, metadata: dict = None):
    # Generate both embeddings
    dense_embedding = get_dense_embedding(text)
    sparse_embedding = splade.encode(text)

    point = Point(
        id=doc_id,
        named_vectors={
            "dense": dense_embedding,
        },
        sparse_vector=SparseVector.create(
            sparse_embedding["indices"],
            sparse_embedding["values"]
        ),
        payload={
            "text": text,
            **(metadata or {})
        }
    )

    client.upsert("documents", [point])


# Index sample documents
documents = [
    ("doc1", "Machine learning algorithms process data to make predictions"),
    ("doc2", "The TCP/IP protocol stack enables network communication"),
    ("doc3", "Python pandas library provides data manipulation tools"),
    ("doc4", "Error code E-1234 indicates connection timeout"),
]

for doc_id, text in documents:
    index_document(doc_id, text)
```

---

## Fusion Strategies

### Linear Combination (Default)

Combines scores with weighted average:

```
hybrid_score = alpha * dense_score + (1 - alpha) * sparse_score
```

```python
results = client.hybrid_search(
    collection_name="documents",
    query_vector=dense_query,
    sparse_vector=sparse_query,
    limit=10,
    fusion="linear",
    alpha=0.7  # 70% dense, 30% sparse
)
```

### Reciprocal Rank Fusion (RRF)

Combines based on rank positions:

```
RRF_score = sum(1 / (k + rank_i)) for each result list
```

```python
results = client.hybrid_search(
    collection_name="documents",
    query_vector=dense_query,
    sparse_vector=sparse_query,
    limit=10,
    fusion="rrf",
    rrf_k=60  # Ranking constant (default: 60)
)
```

### Comparison

| Fusion Method | Pros | Cons | Best For |
|---------------|------|------|----------|
| Linear | Simple, interpretable | Requires score normalization | When scores are comparable |
| RRF | Rank-based, robust | Loses score magnitude | When scores differ in scale |

---

## Tuning Alpha Parameter

The `alpha` parameter controls the balance between dense and sparse search:

- `alpha = 1.0`: Pure dense search
- `alpha = 0.5`: Equal weight
- `alpha = 0.0`: Pure sparse search

### Finding Optimal Alpha

```python
def evaluate_alpha(test_queries: list, ground_truth: dict, alphas: list[float]):
    """Evaluate different alpha values."""
    results = {}

    for alpha in alphas:
        recalls = []

        for query, query_id in test_queries:
            # Get hybrid results
            hybrid_results = client.hybrid_search(
                collection_name="documents",
                query_vector=get_dense_embedding(query),
                sparse_vector=splade.encode(query),
                limit=10,
                alpha=alpha
            )

            # Calculate recall
            retrieved_ids = {r.id for r in hybrid_results}
            relevant_ids = set(ground_truth[query_id])
            recall = len(retrieved_ids & relevant_ids) / len(relevant_ids)
            recalls.append(recall)

        results[alpha] = sum(recalls) / len(recalls)

    return results


# Test different alpha values
alphas = [0.0, 0.3, 0.5, 0.7, 0.8, 0.9, 1.0]
metrics = evaluate_alpha(test_queries, ground_truth, alphas)

for alpha, recall in sorted(metrics.items()):
    print(f"Alpha {alpha:.1f}: Recall@10 = {recall:.3f}")
```

### Query-Type Adaptive Alpha

```python
def get_adaptive_alpha(query: str) -> float:
    """Adjust alpha based on query characteristics."""
    # Check for specific patterns
    has_codes = bool(re.search(r'[A-Z]{2,}-\d+|0x[0-9a-fA-F]+', query))
    has_quotes = '"' in query
    query_length = len(query.split())

    if has_codes or has_quotes:
        # Favor sparse for exact terms
        return 0.3
    elif query_length <= 3:
        # Short queries often need exact matches
        return 0.5
    else:
        # Longer queries benefit from semantic understanding
        return 0.7


# Use adaptive alpha
query = 'error code E-1234'
alpha = get_adaptive_alpha(query)

results = client.hybrid_search(
    collection_name="documents",
    query_vector=get_dense_embedding(query),
    sparse_vector=splade.encode(query),
    limit=10,
    alpha=alpha
)
```

---

## Advanced Techniques

### 1. Late Interaction (ColBERT-style)

```python
def late_interaction_search(query: str, top_k: int = 10):
    """MaxSim-based late interaction search."""
    # Get token-level embeddings for query
    query_tokens = get_token_embeddings(query)  # [num_tokens, dim]

    # First-pass retrieval
    candidates = client.search(
        collection_name="documents",
        query_vector=mean_pool(query_tokens),
        limit=top_k * 10  # Over-retrieve for reranking
    )

    # Late interaction scoring
    scored = []
    for doc in candidates:
        doc_tokens = get_token_embeddings(doc.payload["text"])

        # MaxSim: for each query token, find max similarity to any doc token
        max_sims = []
        for q_tok in query_tokens:
            sims = [cosine_sim(q_tok, d_tok) for d_tok in doc_tokens]
            max_sims.append(max(sims))

        score = sum(max_sims)
        scored.append((doc, score))

    # Return top-k by late interaction score
    scored.sort(key=lambda x: x[1], reverse=True)
    return [doc for doc, _ in scored[:top_k]]
```

### 2. Query Expansion

```python
def expand_query(query: str, num_terms: int = 5) -> str:
    """Expand query with related terms."""
    # Get similar terms from embedding space
    query_vec = get_dense_embedding(query)

    # Search for similar terms in vocabulary index
    similar = client.search(
        collection_name="vocabulary",
        query_vector=query_vec,
        limit=num_terms
    )

    expanded_terms = [r.payload["term"] for r in similar]
    return query + " " + " ".join(expanded_terms)


# Use expanded query for sparse search
original_query = "machine learning"
expanded_query = expand_query(original_query)
# "machine learning neural networks deep learning AI algorithms"
```

### 3. Document Expansion (doc2query)

```python
def expand_document(text: str) -> str:
    """Generate potential queries for a document."""
    # Use a doc2query model
    from transformers import T5ForConditionalGeneration, T5Tokenizer

    model = T5ForConditionalGeneration.from_pretrained("doc2query/msmarco-t5-base-v1")
    tokenizer = T5Tokenizer.from_pretrained("doc2query/msmarco-t5-base-v1")

    inputs = tokenizer.encode(text, return_tensors="pt", max_length=512, truncation=True)
    outputs = model.generate(inputs, max_length=64, num_return_sequences=5)

    queries = [tokenizer.decode(o, skip_special_tokens=True) for o in outputs]
    return text + " " + " ".join(queries)
```

### 4. Learned Sparse Representations

```python
def get_learned_sparse(text: str) -> dict:
    """Use SPLADE for learned sparse representations."""
    from transformers import AutoModelForMaskedLM, AutoTokenizer
    import torch

    tokenizer = AutoTokenizer.from_pretrained("naver/splade-cocondenser-ensembledistil")
    model = AutoModelForMaskedLM.from_pretrained("naver/splade-cocondenser-ensembledistil")

    tokens = tokenizer(text, return_tensors="pt", truncation=True, max_length=512)

    with torch.no_grad():
        output = model(**tokens)

    # SPLADE weighting
    weights = torch.max(
        torch.log1p(torch.relu(output.logits)),
        dim=1
    )[0].squeeze()

    # Get non-zero indices and values
    non_zero = (weights > 0).nonzero().squeeze()
    indices = non_zero.tolist() if non_zero.dim() > 0 else [non_zero.item()]
    values = weights[non_zero].tolist() if non_zero.dim() > 0 else [weights[non_zero].item()]

    return {"indices": indices, "values": values}
```

---

## Benchmarking

### Create Evaluation Dataset

```python
import json
from typing import List, Dict

def create_benchmark(
    queries: List[str],
    relevant_docs: Dict[str, List[str]],
    collection_name: str
) -> Dict:
    """Create a benchmark dataset."""
    return {
        "collection": collection_name,
        "queries": [
            {
                "id": f"q{i}",
                "text": q,
                "relevant": relevant_docs.get(f"q{i}", [])
            }
            for i, q in enumerate(queries)
        ]
    }


# Save benchmark
benchmark = create_benchmark(queries, relevant_docs, "documents")
with open("benchmark.json", "w") as f:
    json.dump(benchmark, f)
```

### Run Benchmark

```python
def run_benchmark(benchmark: Dict, search_fn, k: int = 10) -> Dict:
    """Run benchmark and calculate metrics."""
    metrics = {
        "recall@k": [],
        "precision@k": [],
        "mrr": [],
        "ndcg@k": []
    }

    for query in benchmark["queries"]:
        results = search_fn(query["text"], k)
        retrieved = [r.id for r in results]
        relevant = set(query["relevant"])

        # Recall@k
        recall = len(set(retrieved) & relevant) / len(relevant) if relevant else 0
        metrics["recall@k"].append(recall)

        # Precision@k
        precision = len(set(retrieved) & relevant) / k
        metrics["precision@k"].append(precision)

        # MRR (Mean Reciprocal Rank)
        mrr = 0
        for i, doc_id in enumerate(retrieved):
            if doc_id in relevant:
                mrr = 1 / (i + 1)
                break
        metrics["mrr"].append(mrr)

        # NDCG@k
        dcg = sum(
            1 / math.log2(i + 2) if doc_id in relevant else 0
            for i, doc_id in enumerate(retrieved)
        )
        idcg = sum(1 / math.log2(i + 2) for i in range(min(k, len(relevant))))
        ndcg = dcg / idcg if idcg > 0 else 0
        metrics["ndcg@k"].append(ndcg)

    # Average metrics
    return {k: sum(v) / len(v) for k, v in metrics.items()}
```

### Compare Search Methods

```python
def compare_methods(benchmark: Dict):
    """Compare different search methods."""
    methods = {
        "Dense Only": lambda q, k: client.search(
            "documents", get_dense_embedding(q), limit=k
        ),
        "Sparse Only": lambda q, k: client.search(
            "documents", sparse_vector=splade.encode(q), limit=k
        ),
        "Hybrid (α=0.5)": lambda q, k: client.hybrid_search(
            "documents",
            get_dense_embedding(q),
            splade.encode(q),
            limit=k,
            alpha=0.5
        ),
        "Hybrid (α=0.7)": lambda q, k: client.hybrid_search(
            "documents",
            get_dense_embedding(q),
            splade.encode(q),
            limit=k,
            alpha=0.7
        ),
    }

    results = {}
    for name, search_fn in methods.items():
        metrics = run_benchmark(benchmark, search_fn, k=10)
        results[name] = metrics
        print(f"\n{name}:")
        for metric, value in metrics.items():
            print(f"  {metric}: {value:.4f}")

    return results
```

---

## Best Practices

### 1. Choose the Right Sparse Model

| Model | Quality | Speed | Use Case |
|-------|---------|-------|----------|
| BM25 | Good | Fast | Simple keyword matching |
| SPLADE | Better | Medium | Learned term importance |
| SPLADE++ | Best | Slower | Maximum quality |

### 2. Normalize Scores

```python
def normalize_scores(results: list, method: str = "minmax"):
    """Normalize scores for fair combination."""
    scores = [r.score for r in results]

    if method == "minmax":
        min_s, max_s = min(scores), max(scores)
        if max_s > min_s:
            for r in results:
                r.score = (r.score - min_s) / (max_s - min_s)
    elif method == "zscore":
        mean_s = sum(scores) / len(scores)
        std_s = (sum((s - mean_s) ** 2 for s in scores) / len(scores)) ** 0.5
        if std_s > 0:
            for r in results:
                r.score = (r.score - mean_s) / std_s

    return results
```

### 3. Handle Edge Cases

```python
def safe_hybrid_search(query: str, top_k: int = 10, alpha: float = 0.7):
    """Hybrid search with fallbacks."""
    dense_vec = get_dense_embedding(query)
    sparse_vec = splade.encode(query)

    # Check for empty sparse vector
    if not sparse_vec["indices"]:
        # Fall back to dense only
        return client.search("documents", dense_vec, limit=top_k)

    try:
        return client.hybrid_search(
            "documents",
            dense_vec,
            sparse_vec,
            limit=top_k,
            alpha=alpha
        )
    except Exception as e:
        # Fall back to dense only
        logger.warning(f"Hybrid search failed: {e}, falling back to dense")
        return client.search("documents", dense_vec, limit=top_k)
```

### 4. Cache Embeddings

```python
from functools import lru_cache
import hashlib

@lru_cache(maxsize=10000)
def cached_dense_embed(text: str) -> tuple:
    return tuple(get_dense_embedding(text))

@lru_cache(maxsize=10000)
def cached_sparse_embed(text: str) -> tuple:
    sparse = splade.encode(text)
    return (tuple(sparse["indices"]), tuple(sparse["values"]))
```

### 5. Monitor Performance

```python
import time

def monitored_hybrid_search(query: str, **kwargs):
    """Search with monitoring."""
    start = time.time()

    # Dense embedding
    dense_start = time.time()
    dense_vec = get_dense_embedding(query)
    dense_time = time.time() - dense_start

    # Sparse embedding
    sparse_start = time.time()
    sparse_vec = splade.encode(query)
    sparse_time = time.time() - sparse_start

    # Search
    search_start = time.time()
    results = client.hybrid_search(
        "documents",
        dense_vec,
        sparse_vec,
        **kwargs
    )
    search_time = time.time() - search_start

    total_time = time.time() - start

    logger.info("Hybrid search completed", extra={
        "dense_embedding_ms": dense_time * 1000,
        "sparse_embedding_ms": sparse_time * 1000,
        "search_ms": search_time * 1000,
        "total_ms": total_time * 1000,
        "num_results": len(results)
    })

    return results
```

---

## Next Steps

- [Scaling to Millions](scaling_to_millions.md) - Handle large-scale hybrid search
- [Performance Tuning](../performance_tuning.md) - Optimize search performance
- [Quantization Guide](../quantization.md) - Reduce memory usage

