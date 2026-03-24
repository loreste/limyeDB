"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.LimyeDBClient = void 0;
const axios_1 = __importDefault(require("axios"));
class LimyeDBClient {
    client;
    constructor(host = 'http://localhost:8080', authToken) {
        const headers = {
            'Content-Type': 'application/json',
        };
        if (authToken) {
            headers['Authorization'] = `Bearer ${authToken}`;
        }
        this.client = axios_1.default.create({
            baseURL: host,
            timeout: 30000,
            headers,
        });
    }
    async createCollection(config) {
        await this.client.post('/collections', {
            ...config,
            metric: config.metric || 'cosine',
        });
    }
    async deleteCollection(name) {
        await this.client.delete(`/collections/${name}`);
    }
    async upsert(collectionName, points) {
        await this.client.put(`/collections/${collectionName}/points`, { points });
    }
    async deletePoints(collectionName, ids) {
        await this.client.delete(`/collections/${collectionName}/points`, {
            data: { ids },
        });
    }
    async search(collectionName, vector, limit = 10, filter) {
        const response = await this.client.post(`/collections/${collectionName}/search`, {
            vector,
            limit,
            filter,
        });
        return response.data.result || response.data.points || [];
    }
    async discover(collectionName, params) {
        const response = await this.client.post(`/collections/${collectionName}/discover`, {
            ...params,
            limit: params.limit || 10,
        });
        return response.data.points || [];
    }
    async groupSearch(collectionName, vector, groupBy, groupSize = 3, limit = 10, withVector = false) {
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
exports.LimyeDBClient = LimyeDBClient;
