# LimyeDB Rust Client

Official Rust client for [LimyeDB](https://github.com/loreste/limyeDB) - Enterprise Distributed Vector Database for AI & RAG.

## Installation

Add to your `Cargo.toml`:

```toml
[dependencies]
limyedb = "1.0"
tokio = { version = "1.0", features = ["full"] }
```

## Quick Start

```rust
use limyedb::{LimyeDBClient, Point, CreateCollectionRequest};

#[tokio::main]
async fn main() -> Result<(), limyedb::Error> {
    // Connect to LimyeDB
    let client = LimyeDBClient::new("http://localhost:8080", Some("your-api-key"))?;

    // Create a collection
    client.create_collection(CreateCollectionRequest {
        name: "documents".to_string(),
        dimension: 1536,
        metric: Some("cosine".to_string()),
        ..Default::default()
    }).await?;

    // Insert vectors
    client.upsert("documents", vec![
        Point {
            id: "doc1".to_string(),
            vector: vec![0.1, 0.2, 0.3, /* ... 1536 dimensions */],
            payload: Some(serde_json::json!({
                "title": "Introduction to AI",
                "category": "tech"
            })),
        }
    ]).await?;

    // Search
    let results = client.search("documents", vec![0.1, 0.2, 0.3], 10, None).await?;

    for result in results {
        println!("ID: {}, Score: {}", result.id, result.score);
        if let Some(payload) = result.payload {
            println!("Payload: {}", payload);
        }
    }

    Ok(())
}
```

## Features

- Async/await support with Tokio
- Full type safety with Rust's type system
- TLS support (rustls or native-tls)
- Comprehensive error handling

## API Reference

### Client

```rust
// Create client
let client = LimyeDBClient::new("http://localhost:8080", Some("api-key"))?;

// Health check
let health = client.health().await?;
```

### Collections

```rust
// Create
client.create_collection(CreateCollectionRequest {
    name: "my-collection".to_string(),
    dimension: 1536,
    metric: Some("cosine".to_string()),
    hnsw: Some(HNSWConfig {
        m: Some(16),
        ef_construction: Some(200),
        ef_search: Some(100),
    }),
    ..Default::default()
}).await?;

// Get info
let info = client.get_collection("my-collection").await?;

// List all
let collections = client.list_collections().await?;

// Delete
client.delete_collection("my-collection").await?;
```

### Points

```rust
// Upsert
let result = client.upsert("collection", vec![
    Point {
        id: "1".to_string(),
        vector: vec![0.1, 0.2],
        payload: Some(serde_json::json!({"key": "value"})),
    }
]).await?;

// Get
let point = client.get_point("collection", "1").await?;

// Delete
client.delete_points("collection", vec!["1".to_string()]).await?;
```

### Search

```rust
// Simple search
let results = client.search("collection", vector, 10, None).await?;

// With filter
let filter = Filter {
    must: Some(vec![
        Condition {
            key: "category".to_string(),
            r#match: Some(MatchCondition {
                value: serde_json::json!("tech"),
            }),
            range: None,
        }
    ]),
    must_not: None,
    should: None,
};

let results = client.search("collection", vector, 10, Some(filter)).await?;
```

## License

GPL-3.0
