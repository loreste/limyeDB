export type VectorType = number[] | {
    [key: string]: number[];
};
export interface CollectionConfig {
    name: string;
    dimension: number;
    metric?: string;
}
export interface Point {
    id: string;
    vector: VectorType;
    payload?: Record<string, any>;
}
export interface Match {
    id: string;
    score: number;
    vector?: VectorType;
    payload?: Record<string, any>;
}
export interface ContextExample {
    id?: string;
    vector?: VectorType;
}
export interface ContextPair {
    positive: ContextExample[];
    negative?: ContextExample[];
}
export interface DiscoverParams {
    target?: VectorType;
    context?: ContextPair;
    limit?: number;
    ef?: number;
    filter?: Record<string, any>;
}
export declare class LimyeDBClient {
    private client;
    constructor(host?: string, authToken?: string);
    createCollection(config: CollectionConfig): Promise<void>;
    deleteCollection(name: string): Promise<void>;
    upsert(collectionName: string, points: Point[]): Promise<void>;
    deletePoints(collectionName: string, ids: string[]): Promise<void>;
    search(collectionName: string, vector: VectorType, limit?: number, filter?: Record<string, any>): Promise<Match[]>;
    discover(collectionName: string, params: DiscoverParams): Promise<Match[]>;
    groupSearch(collectionName: string, vector: VectorType, groupBy: string, groupSize?: number, limit?: number, withVector?: boolean): Promise<any>;
}
