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
export declare class LimyeDBClient {
    private client;
    constructor(host?: string);
    createCollection(config: CollectionConfig): Promise<void>;
    deleteCollection(name: string): Promise<void>;
    upsert(collectionName: string, points: Point[]): Promise<void>;
    deletePoints(collectionName: string, ids: string[]): Promise<void>;
    search(collectionName: string, vector: number[], limit?: number, filter?: Record<string, any>): Promise<Match[]>;
    discover(collectionName: string, params: DiscoverParams): Promise<Match[]>;
    groupSearch(collectionName: string, vector: number[], groupBy: string, groupSize?: number, limit?: number, withVector?: boolean): Promise<any>;
}
