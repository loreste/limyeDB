# LimyeDB Client SDK Reference

The repository ships REST/gRPC client SDKs in `clients/` for several
languages. They are thin wrappers over the public API and have not yet
been published to language-specific package registries — install them
directly from the source tree.

| Language | Path |
|----------|------|
| Go | `clients/go/limyedb` |
| Python | `clients/python/` |
| TypeScript | `clients/typescript/limyedb` |
| JavaScript | `clients/javascript` |
| Rust | `clients/rust` |
| C# | `clients/csharp` |

> The Java directory under `clients/java/` currently contains only a
> `pom.xml` and no source files — it is a placeholder, not a working SDK.

All SDKs accept the bearer token from `-auth-token` as the API key. The
same token is the JWT signing key; clients can mint their own JWTs with
HS256 if they want per-collection ACLs (see the
[Security section of the README](../README.md#security)).

## Go

```go
import "github.com/limyedb/limyedb/clients/go/limyedb"

// Against a secured server, pass the bearer token:
client := limyedb.NewClient("http://127.0.0.1:8080", limyedb.WithAuthToken("<auth-token>"))
// Against a server started with -allow-anonymous, the token is unnecessary:
// client := limyedb.NewClient("http://127.0.0.1:8080")
err := client.Upsert("docs", []limyedb.Point{
    {ID: "p1", Vector: []float32{0.5, 0.6}},
})
```

## Python

```python
from limyedb import LimyeDBClient

client = LimyeDBClient(host="http://localhost:8080", api_key="<auth-token>")
client.upsert("docs", [{"id": "p1", "vector": [0.1, 0.2, ...]}])
results = client.search("docs", [0.1, 0.2, ...], limit=10)
```

## JavaScript / TypeScript

```typescript
import { LimyeDBClient } from 'limyedb'

const client = new LimyeDBClient({
  host: 'http://localhost:8080',
  apiKey: '<auth-token>',
})

await client.upsert('docs', [{ id: '1', vector: [0.1, 0.2, ...] }])
const results = await client.search('docs', [0.1, 0.2, ...], 10)
```

The TypeScript client (`clients/typescript/limyedb`) and the JavaScript
client (`clients/javascript`) are separate packages and not all methods
exist in both — TypeScript covers a smaller surface today.

## API surface

All SDKs proxy the REST API documented at:

- [REST endpoints in the README](../README.md#api-quick-reference)
- [gRPC API](grpc_api.md)

If a feature you want isn't in the SDK, the underlying REST/gRPC call
will be — drop down to that until the SDK catches up.
