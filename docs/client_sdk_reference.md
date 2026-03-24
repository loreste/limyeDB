# LimyeDB Client SDK Reference

LimyeDB supports rich, strongly-typed Native SDKs across several enterprise ecosystems natively encapsulating connection pooling, gRPC protocols, and cluster failover mechanisms.

## Node.js / TypeScript SDK

The TypeScript wrapper exposes `LimyeDBClient` enabling generic mappings to local vector stores directly through HTTP wrappers gracefully mimicking native object databases.

```bash
npm install limyedb
```

```typescript
import { LimyeDBClient } from 'limyedb';

const client = new LimyeDBClient({ host: 'localhost', port: 8080 });

await client.upsert('documents', [
  { id: '1', vector: [0.1, 0.2], payload: { name: 'Introduction' } }
]);

const matches = await client.search('documents', [0.1, 0.2], 10, {
  name: { $eq: 'Introduction' }
});
console.log(matches);
```

## Python SDK (With LangChain Support)

Python supports deep semantic integration and native `langchain` integrations straight from pip buffers.

```bash
pip install limyedb
```

```python
from limyedb import LimyeDBClient
from langchain_limyedb import LimyeDBContext

client = LimyeDBClient(host="localhost", port=8080)

# Native Python
client.upsert("docs", [{"id": "p1", "vector": [0.1], "payload": {}}])

# LangChain VectorStore usage
vectorstore = LimyeDBContext(client=client, collection="docs", embeddings=OpenAIEmbeddings())
vectorstore.add_texts(["Hello Semantic Search!"])
print(vectorstore.similarity_search("Hello"))
```

## Go Native SDK
Deploying LimyeDB with Go ensures pure protocol buffers, high resilience throughput and 0 overhead mapping between structures.

```go
import "github.com/limyedb/limyedb/clients/go/limyedb"

client := limyedb.NewClient("127.0.0.1:8080", "optional_api_key")
err := client.Upsert("docs", []limyedb.Point{
    {ID: "p1", Vector: []float32{0.5, 0.6}},
})
```
