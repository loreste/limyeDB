/**
 * LimyeDB JavaScript/TypeScript Client
 *
 * Official client for LimyeDB - Enterprise Distributed Vector Database for AI & RAG.
 *
 * @example
 * ```typescript
 * import { LimyeDBClient } from 'limyedb';
 *
 * const client = new LimyeDBClient({
 *   host: 'localhost',
 *   port: 8080,
 *   apiKey: 'your-api-key'
 * });
 *
 * await client.createCollection({
 *   name: 'documents',
 *   dimension: 1536,
 *   metric: 'cosine'
 * });
 *
 * await client.upsert('documents', [
 *   { id: 'doc1', vector: [0.1, 0.2, ...], payload: { title: 'Hello' } }
 * ]);
 *
 * const results = await client.search('documents', {
 *   vector: [0.1, 0.2, ...],
 *   limit: 10
 * });
 * ```
 */

// Types
export interface LimyeDBConfig {
  host?: string;
  port?: number;
  apiKey?: string;
  https?: boolean;
  timeout?: number;
}

export interface HNSWConfig {
  m?: number;
  ef_construction?: number;
  ef_search?: number;
}

export interface CreateCollectionRequest {
  name: string;
  dimension: number;
  metric?: 'cosine' | 'euclidean' | 'dot_product';
  hnsw_config?: HNSWConfig;
  on_disk?: boolean;
}

export interface Collection {
  name: string;
  dimension: number;
  metric: string;
  points_count: number;
  status: string;
}

export interface Point {
  id: string;
  vector: number[];
  payload?: Record<string, unknown>;
}

export interface SearchRequest {
  vector: number[];
  limit?: number;
  filter?: Filter;
  ef?: number;
  with_payload?: boolean;
  with_vector?: boolean;
  score_threshold?: number;
}

export interface SearchResult {
  id: string;
  score: number;
  vector?: number[];
  payload?: Record<string, unknown>;
}

export interface Filter {
  must?: Condition[];
  must_not?: Condition[];
  should?: Condition[];
}

export interface Condition {
  key?: string;
  match?: { value: unknown };
  range?: { gt?: number; gte?: number; lt?: number; lte?: number };
}

export interface HybridSearchRequest {
  dense_vector: number[];
  sparse_query: string;
  limit?: number;
  filter?: Filter;
  fusion_method?: 'rrf' | 'linear';
  fusion_k?: number;
  with_payload?: boolean;
}

export interface ScrollRequest {
  limit?: number;
  offset?: number;
  filter?: Filter;
  with_payload?: boolean;
  with_vector?: boolean;
}

export interface ScrollResult {
  points: Point[];
  next_offset?: number;
}

// Exceptions
export class LimyeDBError extends Error {
  constructor(message: string, public statusCode?: number) {
    super(message);
    this.name = 'LimyeDBError';
  }
}

export class ConnectionError extends LimyeDBError {
  constructor(message: string) {
    super(message);
    this.name = 'ConnectionError';
  }
}

export class AuthenticationError extends LimyeDBError {
  constructor(message: string) {
    super(message, 401);
    this.name = 'AuthenticationError';
  }
}

export class CollectionNotFoundError extends LimyeDBError {
  constructor(message: string) {
    super(message, 404);
    this.name = 'CollectionNotFoundError';
  }
}

// Client
export class LimyeDBClient {
  private baseUrl: string;
  private headers: Record<string, string>;
  private timeout: number;

  constructor(config: LimyeDBConfig = {}) {
    const {
      host = 'localhost',
      port = 8080,
      apiKey,
      https = false,
      timeout = 30000
    } = config;

    const protocol = https ? 'https' : 'http';
    this.baseUrl = `${protocol}://${host}:${port}`;
    this.timeout = timeout;

    this.headers = {
      'Content-Type': 'application/json',
      'Accept': 'application/json'
    };

    if (apiKey) {
      this.headers['Authorization'] = `Bearer ${apiKey}`;
    }
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown
  ): Promise<T> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    try {
      const response = await fetch(`${this.baseUrl}${path}`, {
        method,
        headers: this.headers,
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal
      });

      clearTimeout(timeoutId);

      if (response.status === 401) {
        throw new AuthenticationError('Invalid API key');
      }

      if (response.status === 404) {
        const text = await response.text();
        if (text.toLowerCase().includes('collection')) {
          throw new CollectionNotFoundError(text);
        }
        throw new LimyeDBError(text, 404);
      }

      if (!response.ok) {
        const text = await response.text();
        throw new LimyeDBError(text, response.status);
      }

      const text = await response.text();
      return text ? JSON.parse(text) : ({} as T);
    } catch (error) {
      clearTimeout(timeoutId);
      if (error instanceof LimyeDBError) {
        throw error;
      }
      if (error instanceof Error && error.name === 'AbortError') {
        throw new ConnectionError('Request timed out');
      }
      throw new ConnectionError(`Failed to connect: ${error}`);
    }
  }

  // Collection operations

  async createCollection(config: CreateCollectionRequest): Promise<Collection> {
    return this.request<Collection>('POST', '/collections', config);
  }

  async getCollection(name: string): Promise<Collection> {
    return this.request<Collection>('GET', `/collections/${name}`);
  }

  async listCollections(): Promise<{ collections: Collection[] }> {
    return this.request<{ collections: Collection[] }>('GET', '/collections');
  }

  async deleteCollection(name: string): Promise<void> {
    await this.request<void>('DELETE', `/collections/${name}`);
  }

  async collectionExists(name: string): Promise<boolean> {
    try {
      await this.getCollection(name);
      return true;
    } catch (error) {
      if (error instanceof CollectionNotFoundError) {
        return false;
      }
      throw error;
    }
  }

  // Point operations

  async upsert(
    collection: string,
    points: Point[],
    wait = true
  ): Promise<{ succeeded: number; failed: number }> {
    return this.request<{ succeeded: number; failed: number }>(
      'PUT',
      `/collections/${collection}/points`,
      { points, wait }
    );
  }

  async upsertBatch(
    collection: string,
    points: Point[],
    batchSize = 100
  ): Promise<{ succeeded: number; failed: number }[]> {
    const results: { succeeded: number; failed: number }[] = [];

    for (let i = 0; i < points.length; i += batchSize) {
      const batch = points.slice(i, i + batchSize);
      const result = await this.upsert(collection, batch);
      results.push(result);
    }

    return results;
  }

  async getPoint(
    collection: string,
    pointId: string,
    options: { with_vector?: boolean; with_payload?: boolean } = {}
  ): Promise<Point> {
    const params = new URLSearchParams();
    if (options.with_vector !== undefined) {
      params.set('with_vector', String(options.with_vector));
    }
    if (options.with_payload !== undefined) {
      params.set('with_payload', String(options.with_payload));
    }
    const query = params.toString();
    const path = `/collections/${collection}/points/${pointId}${query ? '?' + query : ''}`;
    return this.request<Point>('GET', path);
  }

  async getPoints(
    collection: string,
    ids: string[],
    options: { with_vector?: boolean; with_payload?: boolean } = {}
  ): Promise<{ points: Point[] }> {
    return this.request<{ points: Point[] }>(
      'POST',
      `/collections/${collection}/points/get`,
      { ids, ...options }
    );
  }

  async deletePoints(collection: string, ids: string[]): Promise<{ deleted: number }> {
    return this.request<{ deleted: number }>(
      'POST',
      `/collections/${collection}/points/delete`,
      { ids }
    );
  }

  // Search operations

  async search(
    collection: string,
    request: SearchRequest
  ): Promise<{ result: SearchResult[]; took_ms: number }> {
    const payload = {
      vector: request.vector,
      limit: request.limit ?? 10,
      ef: request.ef ?? 100,
      with_payload: request.with_payload ?? true,
      with_vector: request.with_vector ?? false,
      ...(request.filter && { filter: request.filter }),
      ...(request.score_threshold !== undefined && { score_threshold: request.score_threshold })
    };

    return this.request<{ result: SearchResult[]; took_ms: number }>(
      'POST',
      `/collections/${collection}/search`,
      payload
    );
  }

  async searchBatch(
    collection: string,
    vectors: number[][],
    options: Omit<SearchRequest, 'vector'> = {}
  ): Promise<{ results: { result: SearchResult[] }[] }> {
    const searches = vectors.map(vector => ({
      vector,
      limit: options.limit ?? 10,
      with_payload: options.with_payload ?? true,
      ...(options.filter && { filter: options.filter })
    }));

    return this.request<{ results: { result: SearchResult[] }[] }>(
      'POST',
      `/collections/${collection}/search/batch`,
      { searches }
    );
  }

  async hybridSearch(
    collection: string,
    request: HybridSearchRequest
  ): Promise<{ results: SearchResult[]; took_ms: number }> {
    const payload = {
      dense_vector: request.dense_vector,
      sparse_query: request.sparse_query,
      limit: request.limit ?? 10,
      with_payload: request.with_payload ?? true,
      fusion: {
        method: request.fusion_method ?? 'rrf',
        k: request.fusion_k ?? 60
      },
      ...(request.filter && { filter: request.filter })
    };

    return this.request<{ results: SearchResult[]; took_ms: number }>(
      'POST',
      `/collections/${collection}/search/hybrid`,
      payload
    );
  }

  async scroll(
    collection: string,
    request: ScrollRequest = {}
  ): Promise<ScrollResult> {
    return this.request<ScrollResult>(
      'POST',
      `/collections/${collection}/points/scroll`,
      {
        limit: request.limit ?? 100,
        with_payload: request.with_payload ?? true,
        with_vector: request.with_vector ?? false,
        ...(request.offset !== undefined && { offset: request.offset }),
        ...(request.filter && { filter: request.filter })
      }
    );
  }

  // Utility methods

  async health(): Promise<{ status: string; version: string }> {
    return this.request<{ status: string; version: string }>('GET', '/health');
  }

  async info(): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', '/info');
  }
}

// Default export
export default LimyeDBClient;
