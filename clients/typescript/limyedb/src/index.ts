import axios, { AxiosInstance } from 'axios';

export interface CollectionConfig {
  name: string;
  dimension: number;
  metric?: string;
}

export interface Point {
  id: string;
  vector: number[];
  payload?: Record<string, any>;
}

export interface Match {
  id: string;
  score: number;
  vector?: number[];
  payload?: Record<string, any>;
}

export interface ContextExample {
  id?: string;
  vector?: number[];
}

export interface ContextPair {
  positive: ContextExample[];
  negative?: ContextExample[];
}

export interface DiscoverParams {
  target?: number[];
  context?: ContextPair;
  limit?: number;
  ef?: number;
  filter?: Record<string, any>;
}

export class LimyeDBClient {
  private client: AxiosInstance;

  constructor(host: string = 'http://localhost:8080') {
    this.client = axios.create({
      baseURL: host,
      timeout: 30000,
      headers: {
        'Content-Type': 'application/json',
      },
    });
  }

  async createCollection(config: CollectionConfig): Promise<void> {
    await this.client.post('/collections', {
      ...config,
      metric: config.metric || 'cosine',
    });
  }

  async deleteCollection(name: string): Promise<void> {
    await this.client.delete(`/collections/${name}`);
  }

  async upsert(collectionName: string, points: Point[]): Promise<void> {
    await this.client.put(`/collections/${collectionName}/points`, { points });
  }

  async deletePoints(collectionName: string, ids: string[]): Promise<void> {
    await this.client.delete(`/collections/${collectionName}/points`, {
      data: { ids },
    });
  }

  async search(
    collectionName: string,
    vector: number[],
    limit: number = 10,
    filter?: Record<string, any>
  ): Promise<Match[]> {
    const response = await this.client.post(`/collections/${collectionName}/search`, {
      vector,
      limit,
      filter,
    });
    return response.data.result || response.data.points || [];
  }

  async discover(collectionName: string, params: DiscoverParams): Promise<Match[]> {
    const response = await this.client.post(`/collections/${collectionName}/discover`, {
      ...params,
      limit: params.limit || 10,
    });
    return response.data.points || [];
  }

  async groupSearch(
    collectionName: string,
    vector: number[],
    groupBy: string,
    groupSize: number = 3,
    limit: number = 10,
    withVector: boolean = false
  ): Promise<any> {
    const response = await this.client.post(`/collections/${collectionName}/search/groups`, {
      vector,
      group_by: groupBy,
      group_size: groupSize,
      limit,
      with_vector: withVector,
    });
    return response.data;
  }
}
