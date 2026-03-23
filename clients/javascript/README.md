# LimyeDB JavaScript/TypeScript Client

Official JavaScript/TypeScript client for [LimyeDB](https://github.com/loreste/limyeDB) - Enterprise Distributed Vector Database for AI & RAG.

## Installation

```bash
npm install limyedb
# or
yarn add limyedb
# or
pnpm add limyedb
```

## Quick Start

```typescript
import { LimyeDBClient } from 'limyedb';

// Connect to LimyeDB
const client = new LimyeDBClient({
  host: 'localhost',
  port: 8080,
  apiKey: 'your-api-key' // Optional
});

// Create a collection
await client.createCollection({
  name: 'documents',
  dimension: 1536,
  metric: 'cosine'
});

// Insert vectors
await client.upsert('documents', [
  {
    id: 'doc1',
    vector: [0.1, 0.2, 0.3, /* ... 1536 dimensions */],
    payload: { title: 'Introduction to AI', category: 'tech' }
  },
  {
    id: 'doc2',
    vector: [0.4, 0.5, 0.6, /* ... */],
    payload: { title: 'Machine Learning', category: 'tech' }
  }
]);

// Search for similar vectors
const { result } = await client.search('documents', {
  vector: [0.1, 0.2, 0.3, /* ... */],
  limit: 10
});

for (const match of result) {
  console.log(`ID: ${match.id}, Score: ${match.score}`);
  console.log(`Payload:`, match.payload);
}

// Search with filters
const filtered = await client.search('documents', {
  vector: [0.1, 0.2, 0.3, /* ... */],
  limit: 10,
  filter: {
    must: [
      { key: 'category', match: { value: 'tech' } }
    ]
  }
});

// Hybrid search (dense + sparse)
const hybridResults = await client.hybridSearch('documents', {
  dense_vector: [0.1, 0.2, /* ... */],
  sparse_query: 'machine learning introduction',
  limit: 10
});
```

## Features

- Full TypeScript support with type definitions
- Promise-based async API
- Automatic retry and error handling
- Batch operations for high throughput
- Filtering and hybrid search support

## API Reference

### Client Configuration

```typescript
const client = new LimyeDBClient({
  host: 'localhost',     // Server host
  port: 8080,            // Server port
  apiKey: 'secret',      // Optional API key
  https: false,          // Use HTTPS
  timeout: 30000         // Request timeout in ms
});
```

### Collections

```typescript
// Create collection
await client.createCollection({
  name: 'my-collection',
  dimension: 1536,
  metric: 'cosine', // 'cosine' | 'euclidean' | 'dot_product'
  hnsw_config: {
    m: 16,
    ef_construction: 200,
    ef_search: 100
  }
});

// Get collection info
const info = await client.getCollection('my-collection');

// List collections
const { collections } = await client.listCollections();

// Delete collection
await client.deleteCollection('my-collection');

// Check if collection exists
const exists = await client.collectionExists('my-collection');
```

### Points

```typescript
// Upsert points
await client.upsert('collection', [
  { id: '1', vector: [...], payload: { key: 'value' } }
]);

// Batch upsert
await client.upsertBatch('collection', points, 100); // batch size

// Get point
const point = await client.getPoint('collection', 'point-id');

// Get multiple points
const { points } = await client.getPoints('collection', ['id1', 'id2']);

// Delete points
await client.deletePoints('collection', ['id1', 'id2']);
```

### Search

```typescript
// Vector search
const { result, took_ms } = await client.search('collection', {
  vector: [...],
  limit: 10,
  with_payload: true
});

// Batch search
const { results } = await client.searchBatch('collection', [
  [0.1, 0.2, ...],
  [0.3, 0.4, ...]
], { limit: 10 });

// Hybrid search
const hybrid = await client.hybridSearch('collection', {
  dense_vector: [...],
  sparse_query: 'search text',
  fusion_method: 'rrf',
  fusion_k: 60
});

// Scroll/pagination
const page = await client.scroll('collection', {
  limit: 100,
  offset: 0
});
```

## Error Handling

```typescript
import {
  LimyeDBClient,
  LimyeDBError,
  ConnectionError,
  AuthenticationError,
  CollectionNotFoundError
} from 'limyedb';

try {
  await client.search('collection', { vector: [...] });
} catch (error) {
  if (error instanceof AuthenticationError) {
    console.error('Invalid API key');
  } else if (error instanceof CollectionNotFoundError) {
    console.error('Collection does not exist');
  } else if (error instanceof ConnectionError) {
    console.error('Connection failed');
  }
}
```

## License

GPL-3.0
