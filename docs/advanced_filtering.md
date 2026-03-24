# LimyeDB Advanced Filtration Engine

LimyeDB boasts massive payload filtering layers that run tightly inside the HNSW nearest-neighbor edge traversal.

This allows developers to map MongoDB, GraphQL, and deeply nested logic bounds over `uint32` vectors instantly.

## Simple Threshold Matrix
The query filters can use basic boundary keys:
```json
{
  "filter": {
      "price": { "$gte": 40.5 },
      "category": { "$eq": "Electronics" }
  }
}
```

## Nested Recursive Trees (`$and`, `$or`, `$not`)
You can infinitely bridge complex structural objects across dynamic arrays:

```json
{
  "filter": {
    "$and": [
      { 
         "$or": [
            { "location": { "$eq": "Kinshasa" } },
            { "location": { "$eq": "Paris" } }
         ]
      },
      { "tenant_id": { "$eq": "acme_corp" } },
      { "stock": { "$gt": 0 } }
    ]
  }
}
```

LimyeDB securely traverses these layers in `O(log N)` vector spaces, instantly pruning irrelevant edges without sacrificing strict `float32` similarities! Use the SDK or pure REST engines!
