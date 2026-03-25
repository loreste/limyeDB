# LimyeDB Advanced Filtering Guide

This guide covers advanced filtering capabilities in LimyeDB, including complex queries, performance optimization, and best practices.

## Table of Contents

1. [Filter Overview](#filter-overview)
2. [Filter Operators](#filter-operators)
3. [Complex Queries](#complex-queries)
4. [Payload Schema Design](#payload-schema-design)
5. [Payload Indexing](#payload-indexing)
6. [Performance Optimization](#performance-optimization)
7. [Common Patterns](#common-patterns)
8. [Troubleshooting](#troubleshooting)

---

## Filter Overview

Filters in LimyeDB allow you to combine vector similarity search with metadata-based constraints. Filters are applied during the search process, ensuring only relevant vectors are considered.

LimyeDB's filtering engine runs tightly inside the HNSW nearest-neighbor edge traversal, allowing developers to apply complex logic bounds over vectors instantly.

### Basic Filter Structure

```json
{
  "filter": {
    "must": [],      // All conditions must match (AND)
    "must_not": [],  // No conditions should match (NOT)
    "should": []     // At least one condition should match (OR)
  }
}
```

### Alternative Syntax (MongoDB-style)

LimyeDB also supports MongoDB-style query syntax:

```json
{
  "filter": {
    "$and": [
      {"price": {"$gte": 40.5}},
      {"category": {"$eq": "Electronics"}}
    ]
  }
}
```

### Simple Example

```json
{
  "filter": {
    "must": [
      {"key": "category", "match": {"value": "technology"}}
    ]
  }
}
```

---

## Filter Operators

### Match Operators

#### Exact Match

Matches exact values for strings, numbers, and booleans.

```json
// Standard syntax
{"key": "status", "match": {"value": "active"}}

// MongoDB-style
{"status": {"$eq": "active"}}

// Number match
{"key": "version", "match": {"value": 2}}

// Boolean match
{"key": "is_published", "match": {"value": true}}
```

#### Any Match (IN)

Matches any value in a list.

```json
// Standard syntax
{"key": "category", "match": {"any": ["tech", "science", "health"]}}

// MongoDB-style
{"category": {"$in": ["tech", "science", "health"]}}
```

#### Except Match (NOT IN)

Matches any value NOT in the list.

```json
// Standard syntax
{"key": "status", "match": {"except": ["deleted", "archived"]}}

// MongoDB-style
{"status": {"$nin": ["deleted", "archived"]}}
```

### Range Operators

For numeric comparisons.

```json
// Greater than
{"key": "price", "range": {"gt": 100}}
// MongoDB-style: {"price": {"$gt": 100}}

// Greater than or equal
{"key": "price", "range": {"gte": 100}}
// MongoDB-style: {"price": {"$gte": 100}}

// Less than
{"key": "price", "range": {"lt": 1000}}
// MongoDB-style: {"price": {"$lt": 1000}}

// Less than or equal
{"key": "price", "range": {"lte": 1000}}
// MongoDB-style: {"price": {"$lte": 1000}}

// Between (inclusive)
{"key": "price", "range": {"gte": 100, "lte": 1000}}

// Between (exclusive)
{"key": "price", "range": {"gt": 100, "lt": 1000}}
```

### Text Operators

#### Text Match

Matches text containing a substring (case-insensitive).

```json
{"key": "title", "text": {"contains": "machine learning"}}
```

#### Prefix Match

Matches text starting with a prefix.

```json
{"key": "sku", "text": {"prefix": "PROD-"}}
```

#### Suffix Match

Matches text ending with a suffix.

```json
{"key": "email", "text": {"suffix": "@company.com"}}
```

### Geo Operators

For geographic filtering.

```json
// Points within radius
{
  "key": "location",
  "geo": {
    "center": {"lat": 40.7128, "lon": -74.0060},
    "radius": 10000  // meters
  }
}

// Points within bounding box
{
  "key": "location",
  "geo": {
    "bounding_box": {
      "top_left": {"lat": 41.0, "lon": -75.0},
      "bottom_right": {"lat": 40.0, "lon": -73.0}
    }
  }
}
```

### Array Operators

For filtering on array fields.

```json
// Array contains value
{"key": "tags", "match": {"value": "featured"}}

// Array contains any of
{"key": "tags", "match": {"any": ["featured", "trending"]}}

// Array contains all of
{"key": "tags", "all": ["featured", "verified"]}

// Array length
{"key": "tags", "count": {"gte": 3}}
```

### Null/Exists Operators

```json
// Field exists and is not null
{"key": "description", "exists": true}
// MongoDB-style: {"description": {"$exists": true}}

// Field does not exist or is null
{"key": "description", "exists": false}

// Field is empty (null, [], "")
{"key": "content", "is_empty": true}
```

### Date/Time Operators

Dates should be stored as ISO 8601 strings or Unix timestamps.

```json
// ISO 8601 string comparison
{
  "key": "created_at",
  "range": {
    "gte": "2024-01-01T00:00:00Z",
    "lt": "2024-02-01T00:00:00Z"
  }
}

// Unix timestamp (seconds)
{
  "key": "timestamp",
  "range": {
    "gte": 1704067200,
    "lt": 1706745600
  }
}
```

---

## Complex Queries

### Combining Must, Must Not, Should

```json
{
  "filter": {
    "must": [
      {"key": "type", "match": {"value": "article"}},
      {"key": "published", "match": {"value": true}}
    ],
    "must_not": [
      {"key": "status", "match": {"value": "deleted"}}
    ],
    "should": [
      {"key": "featured", "match": {"value": true}},
      {"key": "trending", "match": {"value": true}}
    ],
    "min_should_match": 1
  }
}
```

### Nested Recursive Trees ($and, $or, $not)

You can build complex structural queries across dynamic arrays:

```json
{
  "filter": {
    "$and": [
      {
         "$or": [
            {"location": {"$eq": "Kinshasa"}},
            {"location": {"$eq": "Paris"}}
         ]
      },
      {"tenant_id": {"$eq": "acme_corp"}},
      {"stock": {"$gt": 0}}
    ]
  }
}
```

### Nested Conditions (Standard Syntax)

Use nested filters for complex boolean logic.

```json
{
  "filter": {
    "must": [
      {
        "nested": {
          "should": [
            {"key": "category", "match": {"value": "tech"}},
            {"key": "category", "match": {"value": "science"}}
          ]
        }
      },
      {
        "nested": {
          "must": [
            {"key": "price", "range": {"gte": 10}},
            {"key": "price", "range": {"lte": 100}}
          ]
        }
      }
    ]
  }
}
```

### Complex Boolean Logic

**(A AND B) OR (C AND D)**

```json
{
  "filter": {
    "should": [
      {
        "nested": {
          "must": [
            {"key": "type", "match": {"value": "premium"}},
            {"key": "active", "match": {"value": true}}
          ]
        }
      },
      {
        "nested": {
          "must": [
            {"key": "type", "match": {"value": "trial"}},
            {"key": "days_left", "range": {"gt": 0}}
          ]
        }
      }
    ],
    "min_should_match": 1
  }
}
```

**NOT (A OR B)**

```json
{
  "filter": {
    "must_not": [
      {"key": "status", "match": {"value": "deleted"}},
      {"key": "status", "match": {"value": "archived"}}
    ]
  }
}
```

Or using MongoDB-style:

```json
{
  "filter": {
    "$not": {
      "$or": [
        {"status": {"$eq": "deleted"}},
        {"status": {"$eq": "archived"}}
      ]
    }
  }
}
```

---

## Payload Schema Design

### Best Practices

#### 1. Use Flat Structures

```json
// Good - flat structure
{
  "category": "electronics",
  "brand": "Apple",
  "price": 999,
  "in_stock": true
}

// Avoid - deeply nested
{
  "product": {
    "details": {
      "category": "electronics"
    }
  }
}
```

#### 2. Normalize Values

```json
// Good - consistent casing
{
  "status": "active",  // always lowercase
  "category": "Electronics"  // always capitalized
}

// Avoid - inconsistent
{
  "status": "Active",  // mixed case
  "category": "electronics"
}
```

#### 3. Use Appropriate Types

```json
// Good - proper types
{
  "price": 99.99,           // number
  "count": 42,              // integer
  "active": true,           // boolean
  "tags": ["a", "b"],       // array
  "created": "2024-01-15T10:30:00Z"  // ISO date string
}

// Avoid - string for everything
{
  "price": "99.99",
  "count": "42",
  "active": "true"
}
```

#### 4. Design for Query Patterns

```json
// If you filter by date ranges frequently
{
  "created_year": 2024,
  "created_month": 1,
  "created_day": 15,
  "created_at": "2024-01-15T10:30:00Z"
}

// If you filter by category hierarchies
{
  "category_l1": "Electronics",
  "category_l2": "Phones",
  "category_l3": "Smartphones"
}
```

### Schema Examples

#### E-commerce Product

```json
{
  "id": "prod-123",
  "name": "iPhone 15 Pro",
  "category": "smartphones",
  "brand": "apple",
  "price": 999.00,
  "currency": "USD",
  "in_stock": true,
  "stock_count": 150,
  "rating": 4.8,
  "review_count": 2340,
  "tags": ["5g", "wireless-charging", "face-id"],
  "created_at": "2024-01-15T00:00:00Z"
}
```

#### Document/Article

```json
{
  "id": "doc-456",
  "title": "Introduction to Vector Databases",
  "author": "Jane Smith",
  "author_id": "user-789",
  "category": "technology",
  "tags": ["database", "ai", "vectors"],
  "word_count": 2500,
  "reading_time_minutes": 10,
  "published": true,
  "published_at": "2024-03-01T12:00:00Z",
  "language": "en",
  "access_level": "public"
}
```

#### User Profile

```json
{
  "id": "user-123",
  "username": "johndoe",
  "email_domain": "company.com",
  "account_type": "premium",
  "verified": true,
  "created_at": "2023-06-15T00:00:00Z",
  "last_active": "2024-03-20T14:30:00Z",
  "preferences": ["notifications", "dark-mode"],
  "location_country": "US",
  "location_city": "New York"
}
```

---

## Payload Indexing

### Creating Payload Indexes

Indexes dramatically improve filter performance for frequently queried fields.

```bash
# Create keyword index for exact string matches
curl -X PUT http://localhost:8080/collections/products/index \
  -d '{"field_name": "category", "field_schema": "keyword"}'

# Create integer index for numeric ranges
curl -X PUT http://localhost:8080/collections/products/index \
  -d '{"field_name": "price", "field_schema": "integer"}'

# Create float index
curl -X PUT http://localhost:8080/collections/products/index \
  -d '{"field_name": "rating", "field_schema": "float"}'

# Create boolean index
curl -X PUT http://localhost:8080/collections/products/index \
  -d '{"field_name": "in_stock", "field_schema": "bool"}'

# Create geo index
curl -X PUT http://localhost:8080/collections/stores/index \
  -d '{"field_name": "location", "field_schema": "geo"}'

# Create text index (for substring search)
curl -X PUT http://localhost:8080/collections/products/index \
  -d '{"field_name": "description", "field_schema": "text"}'

# Create datetime index
curl -X PUT http://localhost:8080/collections/products/index \
  -d '{"field_name": "created_at", "field_schema": "datetime"}'
```

### Index Types Reference

| Schema Type | Use Case | Operators Supported |
|-------------|----------|---------------------|
| `keyword` | Exact string match | match, any, except, $eq, $in, $nin |
| `integer` | Integer ranges | range (gt, gte, lt, lte), match, $gt, $gte, $lt, $lte |
| `float` | Float ranges | range, match |
| `bool` | Boolean flags | match, $eq |
| `geo` | Geographic queries | geo (radius, bounding_box) |
| `text` | Substring search | text (contains, prefix, suffix) |
| `datetime` | Date/time ranges | range |

### Listing Indexes

```bash
curl http://localhost:8080/collections/products/index
```

### Deleting Indexes

```bash
curl -X DELETE http://localhost:8080/collections/products/index/category
```

---

## Performance Optimization

LimyeDB securely traverses filter layers in `O(log N)` vector spaces, instantly pruning irrelevant edges without sacrificing `float32` similarities.

### Filter Selectivity

Filter selectivity significantly impacts performance:

| Selectivity | Definition | Performance |
|-------------|------------|-------------|
| High (>50%) | Most points match | Faster (post-filter) |
| Medium (10-50%) | Some points match | Adaptive |
| Low (<10%) | Few points match | Slower (pre-filter) |

### Optimization Strategies

#### 1. Index Frequently Filtered Fields

```bash
# Priority: High-frequency, high-selectivity fields
curl -X PUT http://localhost:8080/collections/docs/index \
  -d '{"field_name": "tenant_id", "field_schema": "keyword"}'
```

#### 2. Order Conditions by Selectivity

```json
{
  "filter": {
    "must": [
      // Most selective first (fewer matches)
      {"key": "tenant_id", "match": {"value": "tenant-123"}},
      // Less selective second
      {"key": "status", "match": {"value": "active"}},
      // Least selective last
      {"key": "created_year", "range": {"gte": 2020}}
    ]
  }
}
```

#### 3. Use Specific Operators

```json
// Good - specific match
{"key": "status", "match": {"value": "active"}}

// Less efficient - range for equality
{"key": "status", "range": {"gte": "active", "lte": "active"}}
```

#### 4. Avoid Complex Should Clauses

```json
// Less efficient - many OR conditions
{
  "should": [
    {"key": "cat", "match": {"value": "a"}},
    {"key": "cat", "match": {"value": "b"}},
    {"key": "cat", "match": {"value": "c"}},
    // ... many more
  ]
}

// More efficient - use "any" operator
{
  "must": [
    {"key": "cat", "match": {"any": ["a", "b", "c", ...]}}
  ]
}
```

#### 5. Limit Result Size

```json
{
  "vector": [...],
  "limit": 10,  // Don't request more than needed
  "filter": {...}
}
```

### Performance Benchmarks

| Filter Type | Indexed | Unindexed | Improvement |
|-------------|---------|-----------|-------------|
| Exact match | 2ms | 50ms | 25x |
| Range query | 5ms | 100ms | 20x |
| Text search | 10ms | 200ms | 20x |
| Geo radius | 8ms | 150ms | 19x |

---

## Common Patterns

### Multi-Tenant Filtering

Always filter by tenant for data isolation:

```json
{
  "filter": {
    "must": [
      {"key": "tenant_id", "match": {"value": "tenant-123"}},
      // ... other conditions
    ]
  }
}
```

Or using MongoDB-style:

```json
{
  "filter": {
    "$and": [
      {"tenant_id": {"$eq": "acme_corp"}},
      // ... other conditions
    ]
  }
}
```

### Time-Based Filtering

Filter by time windows:

```json
// Last 24 hours
{
  "filter": {
    "must": [
      {
        "key": "created_at",
        "range": {
          "gte": "2024-03-24T00:00:00Z"
        }
      }
    ]
  }
}

// Specific date range
{
  "filter": {
    "must": [
      {
        "key": "created_at",
        "range": {
          "gte": "2024-01-01T00:00:00Z",
          "lt": "2024-04-01T00:00:00Z"
        }
      }
    ]
  }
}
```

### Access Control

Filter based on user permissions:

```json
{
  "filter": {
    "should": [
      {"key": "access_level", "match": {"value": "public"}},
      {"key": "owner_id", "match": {"value": "current-user-id"}},
      {"key": "shared_with", "match": {"value": "current-user-id"}}
    ],
    "min_should_match": 1
  }
}
```

### Category Hierarchy

Filter by category levels:

```json
// All electronics
{"key": "category_l1", "match": {"value": "Electronics"}}

// All phones
{
  "must": [
    {"key": "category_l1", "match": {"value": "Electronics"}},
    {"key": "category_l2", "match": {"value": "Phones"}}
  ]
}
```

### Tag-Based Filtering

```json
// Has specific tag
{"key": "tags", "match": {"value": "featured"}}

// Has any of these tags
{"key": "tags", "match": {"any": ["featured", "trending", "new"]}}

// Has all of these tags
{
  "must": [
    {"key": "tags", "match": {"value": "verified"}},
    {"key": "tags", "match": {"value": "premium"}}
  ]
}
```

### Price Range with Availability

```json
{
  "filter": {
    "must": [
      {"key": "in_stock", "match": {"value": true}},
      {"key": "price", "range": {"gte": 50, "lte": 200}}
    ]
  }
}
```

### Geographic Filtering

Find stores near a location:

```json
{
  "filter": {
    "must": [
      {
        "key": "location",
        "geo": {
          "center": {"lat": 40.7128, "lon": -74.0060},
          "radius": 5000
        }
      },
      {"key": "open_now", "match": {"value": true}}
    ]
  }
}
```

### Faceted Search

Build facets for UI filters:

```python
# Get counts per category
categories = client.aggregate(
    collection="products",
    field="category",
    filter=base_filter
)

# Get price range
price_stats = client.stats(
    collection="products",
    field="price",
    filter=base_filter
)
```

---

## Troubleshooting

### Filter Returns No Results

1. **Check field names** - case-sensitive
   ```json
   // Wrong
   {"key": "Category", "match": {"value": "tech"}}
   // Correct
   {"key": "category", "match": {"value": "tech"}}
   ```

2. **Check value types**
   ```json
   // Wrong - string instead of number
   {"key": "count", "range": {"gte": "10"}}
   // Correct - number
   {"key": "count", "range": {"gte": 10}}
   ```

3. **Verify data exists**
   ```python
   point = client.get_point("collection", "known-id")
   print(point.payload)  # Check actual field values
   ```

### Filter Returns Wrong Results

1. **Check operator logic**
   - `must` / `$and` = AND (all must match)
   - `should` / `$or` = OR (at least one must match)
   - `must_not` / `$not` = NOT (none should match)

2. **Verify nested structure**
   ```json
   // Each nested block is evaluated independently
   {
     "should": [
       {"nested": {"must": [A, B]}},  // (A AND B)
       {"nested": {"must": [C, D]}}   // OR (C AND D)
     ]
   }
   ```

### Slow Filtered Queries

1. **Check if field is indexed**
   ```bash
   curl http://localhost:8080/collections/name/index
   ```

2. **Create missing indexes**
   ```bash
   curl -X PUT http://localhost:8080/collections/name/index \
     -d '{"field_name": "field", "field_schema": "keyword"}'
   ```

3. **Check filter selectivity** - very selective filters may require full scan

4. **Increase ef** for filtered search
   ```json
   {"vector": [...], "limit": 10, "ef": 200, "filter": {...}}
   ```

### Memory Issues with Large Filters

1. **Avoid very large IN lists**
   ```json
   // Instead of 1000+ values in "any"
   // Consider restructuring data or using multiple queries
   ```

2. **Break complex queries into smaller parts**
   ```python
   # Instead of one complex query
   results = []
   for category in categories:
       partial = client.search(
           collection,
           query,
           filter={"must": [{"key": "cat", "match": {"value": category}}]}
       )
       results.extend(partial)
   ```

### Type Mismatch Errors

1. **Ensure consistent types across documents**
   ```json
   // All documents should use same type for a field
   {"price": 99.99}  // float
   {"price": 100}    // Also treat as float, not int
   ```

2. **Use explicit type conversion when inserting**
   ```python
   point = Point(
       id="1",
       vector=[...],
       payload={
           "price": float(price),  # Always float
           "count": int(count),    # Always int
       }
   )
   ```

---

## SDK Examples

### Python

```python
from limyedb import Filter, Condition

# Simple filter
filter = Filter.must_match("category", "electronics")

# Complex filter
filter = Filter()
filter.add_must(Condition.match("status", "active"))
filter.add_must(Condition.range("price", gte=50, lte=500))
filter.add_must_not(Condition.match("deleted", True))

results = client.search("products", query_vector, limit=10, filter=filter)
```

### JavaScript

```javascript
// Simple filter
const filter = { must: [{ key: "category", match: { value: "electronics" } }] };

// Complex filter
const filter = {
  must: [
    { key: "status", match: { value: "active" } },
    { key: "price", range: { gte: 50, lte: 500 } }
  ],
  must_not: [
    { key: "deleted", match: { value: true } }
  ]
};

const results = await client.search("products", queryVector, { limit: 10, filter });
```

### Go

```go
filter := &Filter{
    Must: []Condition{
        {Key: "status", Match: &Match{Value: "active"}},
        {Key: "price", Range: &Range{Gte: 50, Lte: 500}},
    },
    MustNot: []Condition{
        {Key: "deleted", Match: &Match{Value: true}},
    },
}

results, err := client.Search(ctx, "products", queryVector, 10, filter)
```

