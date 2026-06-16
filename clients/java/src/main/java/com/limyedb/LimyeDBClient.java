package com.limyedb;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.limyedb.exceptions.*;
import com.limyedb.models.*;
import okhttp3.*;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.Closeable;
import java.io.IOException;
import java.util.*;
import java.util.concurrent.TimeUnit;

/**
 * Official Java Client for LimyeDB - Enterprise Distributed Vector Database.
 *
 * <p>Example usage:
 * <pre>{@code
 * try (LimyeDBClient client = new LimyeDBClient("http://localhost:8080", "your-api-key")) {
 *     // Create a collection
 *     client.createCollection(CollectionConfig.builder()
 *         .name("documents")
 *         .dimension(1536)
 *         .metric("cosine")
 *         .build());
 *
 *     // Insert vectors
 *     client.upsert("documents", Arrays.asList(
 *         Point.builder()
 *             .id("doc1")
 *             .vector(Arrays.asList(0.1f, 0.2f, ...))
 *             .payload(Map.of("title", "Hello"))
 *             .build()
 *     ));
 *
 *     // Search
 *     List<SearchResult> results = client.search("documents",
 *         Arrays.asList(0.1f, 0.2f, ...), 10);
 * }
 * }</pre>
 */
public class LimyeDBClient implements Closeable {

    private static final Logger logger = LoggerFactory.getLogger(LimyeDBClient.class);
    private static final MediaType JSON = MediaType.parse("application/json; charset=utf-8");

    private final OkHttpClient httpClient;
    private final ObjectMapper objectMapper;
    private final String baseUrl;

    /**
     * Creates a new LimyeDB client.
     *
     * @param host   Server URL (e.g., "http://localhost:8080")
     * @param apiKey API key for authentication (can be null)
     */
    public LimyeDBClient(String host, String apiKey) {
        this(host, apiKey, 30);
    }

    /**
     * Creates a new LimyeDB client with custom timeout.
     *
     * @param host      Server URL
     * @param apiKey    API key for authentication
     * @param timeoutSeconds  Request timeout in seconds
     */
    public LimyeDBClient(String host, String apiKey, int timeoutSeconds) {
        this.baseUrl = host.endsWith("/") ? host.substring(0, host.length() - 1) : host;

        OkHttpClient.Builder builder = new OkHttpClient.Builder()
                .connectTimeout(timeoutSeconds, TimeUnit.SECONDS)
                .readTimeout(timeoutSeconds, TimeUnit.SECONDS)
                .writeTimeout(timeoutSeconds, TimeUnit.SECONDS);

        if (apiKey != null && !apiKey.isEmpty()) {
            builder.addInterceptor(chain -> {
                Request original = chain.request();
                Request request = original.newBuilder()
                        .header("Authorization", "Bearer " + apiKey)
                        .header("Content-Type", "application/json")
                        .header("Accept", "application/json")
                        .build();
                return chain.proceed(request);
            });
        } else {
            builder.addInterceptor(chain -> {
                Request original = chain.request();
                Request request = original.newBuilder()
                        .header("Content-Type", "application/json")
                        .header("Accept", "application/json")
                        .build();
                return chain.proceed(request);
            });
        }

        this.httpClient = builder.build();
        this.objectMapper = new ObjectMapper()
                .configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false);
    }

    // ==================== Collection Operations ====================

    /**
     * Creates a new collection.
     *
     * @param config Collection configuration
     * @return Collection information
     */
    public CollectionInfo createCollection(CollectionConfig config) throws LimyeDBException {
        String json = toJson(config);
        return post("/collections", json, CollectionInfo.class);
    }

    /**
     * Gets information about a collection.
     *
     * @param name Collection name
     * @return Collection information
     */
    public CollectionInfo getCollection(String name) throws LimyeDBException {
        return get("/collections/" + name, CollectionInfo.class);
    }

    /**
     * Lists all collections.
     *
     * @return List of collection information
     */
    public List<CollectionInfo> listCollections() throws LimyeDBException {
        JsonNode response = get("/collections", JsonNode.class);
        if (response.has("collections")) {
            return objectMapper.convertValue(
                    response.get("collections"),
                    new TypeReference<List<CollectionInfo>>() {}
            );
        }
        return Collections.emptyList();
    }

    /**
     * Deletes a collection.
     *
     * @param name Collection name
     */
    public void deleteCollection(String name) throws LimyeDBException {
        delete("/collections/" + name);
    }

    /**
     * Checks if a collection exists.
     *
     * @param name Collection name
     * @return true if the collection exists
     */
    public boolean collectionExists(String name) {
        try {
            getCollection(name);
            return true;
        } catch (CollectionNotFoundException e) {
            return false;
        } catch (LimyeDBException e) {
            logger.warn("Error checking collection existence: {}", e.getMessage());
            return false;
        }
    }

    // ==================== Point Operations ====================

    /**
     * Upserts points into a collection.
     *
     * @param collectionName Collection name
     * @param points         Points to upsert
     * @return Operation result
     */
    public UpsertResult upsert(String collectionName, List<Point> points) throws LimyeDBException {
        return upsert(collectionName, points, true);
    }

    /**
     * Upserts points into a collection.
     *
     * @param collectionName Collection name
     * @param points         Points to upsert
     * @param wait           Whether to wait for the operation to complete
     * @return Operation result
     */
    public UpsertResult upsert(String collectionName, List<Point> points, boolean wait) throws LimyeDBException {
        Map<String, Object> payload = new HashMap<>();
        payload.put("points", points);
        payload.put("wait", wait);

        String json = toJson(payload);
        return put("/collections/" + collectionName + "/points", json, UpsertResult.class);
    }

    /**
     * Upserts points in batches.
     *
     * @param collectionName Collection name
     * @param points         Points to upsert
     * @param batchSize      Number of points per batch
     * @return List of results for each batch
     */
    public List<UpsertResult> upsertBatch(String collectionName, List<Point> points, int batchSize) throws LimyeDBException {
        List<UpsertResult> results = new ArrayList<>();

        for (int i = 0; i < points.size(); i += batchSize) {
            int end = Math.min(i + batchSize, points.size());
            List<Point> batch = points.subList(i, end);
            results.add(upsert(collectionName, batch));
        }

        return results;
    }

    /**
     * Gets a single point by ID.
     *
     * @param collectionName Collection name
     * @param pointId        Point ID
     * @return The point
     */
    public Point getPoint(String collectionName, String pointId) throws LimyeDBException {
        return getPoint(collectionName, pointId, true, true);
    }

    /**
     * Gets a single point by ID with options.
     *
     * @param collectionName Collection name
     * @param pointId        Point ID
     * @param withVector     Include vector in response
     * @param withPayload    Include payload in response
     * @return The point
     */
    public Point getPoint(String collectionName, String pointId, boolean withVector, boolean withPayload) throws LimyeDBException {
        String url = String.format("/collections/%s/points/%s?with_vector=%b&with_payload=%b",
                collectionName, pointId, withVector, withPayload);
        return get(url, Point.class);
    }

    /**
     * Gets multiple points by IDs.
     *
     * @param collectionName Collection name
     * @param ids            Point IDs
     * @return List of points
     */
    public List<Point> getPoints(String collectionName, List<String> ids) throws LimyeDBException {
        return getPoints(collectionName, ids, true, true);
    }

    /**
     * Gets multiple points by IDs with options.
     *
     * @param collectionName Collection name
     * @param ids            Point IDs
     * @param withVector     Include vectors in response
     * @param withPayload    Include payloads in response
     * @return List of points
     */
    public List<Point> getPoints(String collectionName, List<String> ids, boolean withVector, boolean withPayload) throws LimyeDBException {
        Map<String, Object> payload = new HashMap<>();
        payload.put("ids", ids);
        payload.put("with_vector", withVector);
        payload.put("with_payload", withPayload);

        String json = toJson(payload);
        JsonNode response = post("/collections/" + collectionName + "/points/get", json, JsonNode.class);

        if (response.has("points")) {
            return objectMapper.convertValue(
                    response.get("points"),
                    new TypeReference<List<Point>>() {}
            );
        }
        return Collections.emptyList();
    }

    /**
     * Deletes points by IDs.
     *
     * @param collectionName Collection name
     * @param ids            Point IDs to delete
     * @return Number of deleted points
     */
    public int deletePoints(String collectionName, List<String> ids) throws LimyeDBException {
        Map<String, Object> payload = new HashMap<>();
        payload.put("ids", ids);

        String json = toJson(payload);
        JsonNode response = post("/collections/" + collectionName + "/points/delete", json, JsonNode.class);

        if (response.has("deleted")) {
            return response.get("deleted").asInt();
        }
        return 0;
    }

    // ==================== Search Operations ====================

    /**
     * Searches for similar vectors.
     *
     * @param collectionName Collection name
     * @param vector         Query vector
     * @param limit          Maximum number of results
     * @return List of search results
     */
    public List<SearchResult> search(String collectionName, List<Float> vector, int limit) throws LimyeDBException {
        return search(collectionName, vector, limit, null, 100, true, false);
    }

    /**
     * Searches for similar vectors with filter.
     *
     * @param collectionName Collection name
     * @param vector         Query vector
     * @param limit          Maximum number of results
     * @param filter         Filter conditions
     * @return List of search results
     */
    public List<SearchResult> search(String collectionName, List<Float> vector, int limit, Filter filter) throws LimyeDBException {
        return search(collectionName, vector, limit, filter, 100, true, false);
    }

    /**
     * Searches for similar vectors with full options.
     *
     * @param collectionName Collection name
     * @param vector         Query vector
     * @param limit          Maximum number of results
     * @param filter         Filter conditions
     * @param ef             HNSW ef search parameter
     * @param withPayload    Include payloads in results
     * @param withVector     Include vectors in results
     * @return List of search results
     */
    public List<SearchResult> search(String collectionName, List<Float> vector, int limit,
                                     Filter filter, int ef, boolean withPayload, boolean withVector) throws LimyeDBException {
        Map<String, Object> payload = new HashMap<>();
        payload.put("vector", vector);
        payload.put("limit", limit);
        payload.put("ef", ef);
        payload.put("with_payload", withPayload);
        payload.put("with_vector", withVector);

        if (filter != null) {
            payload.put("filter", filter);
        }

        String json = toJson(payload);
        JsonNode response = post("/collections/" + collectionName + "/search", json, JsonNode.class);

        if (response.has("result")) {
            return objectMapper.convertValue(
                    response.get("result"),
                    new TypeReference<List<SearchResult>>() {}
            );
        }
        return Collections.emptyList();
    }

    /**
     * Performs batch search for multiple queries.
     *
     * @param collectionName Collection name
     * @param vectors        Query vectors
     * @param limit          Maximum results per query
     * @return List of result lists, one per query
     */
    public List<List<SearchResult>> searchBatch(String collectionName, List<List<Float>> vectors, int limit) throws LimyeDBException {
        return searchBatch(collectionName, vectors, limit, null, true);
    }

    /**
     * Performs batch search with filter.
     *
     * @param collectionName Collection name
     * @param vectors        Query vectors
     * @param limit          Maximum results per query
     * @param filter         Filter conditions
     * @param withPayload    Include payloads
     * @return List of result lists
     */
    public List<List<SearchResult>> searchBatch(String collectionName, List<List<Float>> vectors,
                                                int limit, Filter filter, boolean withPayload) throws LimyeDBException {
        List<Map<String, Object>> searches = new ArrayList<>();
        for (List<Float> vector : vectors) {
            Map<String, Object> search = new HashMap<>();
            search.put("vector", vector);
            search.put("limit", limit);
            search.put("with_payload", withPayload);
            if (filter != null) {
                search.put("filter", filter);
            }
            searches.add(search);
        }

        Map<String, Object> payload = new HashMap<>();
        payload.put("searches", searches);

        String json = toJson(payload);
        JsonNode response = post("/collections/" + collectionName + "/search/batch", json, JsonNode.class);

        List<List<SearchResult>> results = new ArrayList<>();
        if (response.has("results")) {
            for (JsonNode batch : response.get("results")) {
                if (batch.has("result")) {
                    List<SearchResult> batchResults = objectMapper.convertValue(
                            batch.get("result"),
                            new TypeReference<List<SearchResult>>() {}
                    );
                    results.add(batchResults);
                } else {
                    results.add(Collections.emptyList());
                }
            }
        }
        return results;
    }

    /**
     * Performs hybrid search combining dense and sparse vectors.
     *
     * @param collectionName Collection name
     * @param denseVector    Dense embedding vector
     * @param sparseQuery    Sparse vector query
     * @param limit          Maximum number of results
     * @return List of search results
     */
    public List<SearchResult> hybridSearch(String collectionName, List<Float> denseVector,
                                           SparseVector sparseQuery, int limit) throws LimyeDBException {
        return hybridSearch(collectionName, denseVector, sparseQuery, limit, null, "rrf", 60, true);
    }

    /**
     * Performs hybrid search with full options.
     *
     * @param collectionName Collection name
     * @param denseVector    Dense embedding vector
     * @param sparseQuery    Sparse vector query
     * @param limit          Maximum number of results
     * @param filter         Filter conditions
     * @param fusionMethod   Fusion method ("rrf" or "linear")
     * @param fusionK        RRF k parameter
     * @param withPayload    Include payloads
     * @return List of search results
     */
    public List<SearchResult> hybridSearch(String collectionName, List<Float> denseVector,
                                           SparseVector sparseQuery, int limit, Filter filter,
                                           String fusionMethod, int fusionK, boolean withPayload) throws LimyeDBException {
        Map<String, Object> payload = new HashMap<>();
        payload.put("limit", limit);
        payload.put("with_payload", withPayload);

        if (denseVector != null) {
            payload.put("dense_vector", denseVector);
        }
        if (sparseQuery != null) {
            payload.put("sparse_query", sparseQuery);
        }
        if (filter != null) {
            payload.put("filter", filter);
        }

        Map<String, Object> fusion = new HashMap<>();
        fusion.put("method", fusionMethod);
        fusion.put("k", fusionK);
        payload.put("fusion", fusion);

        String json = toJson(payload);
        JsonNode response = post("/collections/" + collectionName + "/search/hybrid", json, JsonNode.class);

        if (response.has("results")) {
            return objectMapper.convertValue(
                    response.get("results"),
                    new TypeReference<List<SearchResult>>() {}
            );
        }
        return Collections.emptyList();
    }

    /**
     * Scrolls through points in a collection.
     *
     * @param collectionName Collection name
     * @param limit          Number of points per page
     * @return Scroll result with points and next offset
     */
    public ScrollResult scroll(String collectionName, int limit) throws LimyeDBException {
        return scroll(collectionName, limit, null, null, true, false);
    }

    /**
     * Scrolls through points with options.
     *
     * @param collectionName Collection name
     * @param limit          Number of points per page
     * @param offset         Starting offset
     * @param filter         Filter conditions
     * @param withPayload    Include payloads
     * @param withVector     Include vectors
     * @return Scroll result
     */
    public ScrollResult scroll(String collectionName, int limit, String offset,
                               Filter filter, boolean withPayload, boolean withVector) throws LimyeDBException {
        Map<String, Object> payload = new HashMap<>();
        payload.put("limit", limit);
        payload.put("with_payload", withPayload);
        payload.put("with_vector", withVector);

        if (offset != null) {
            payload.put("offset", offset);
        }
        if (filter != null) {
            payload.put("filter", filter);
        }

        String json = toJson(payload);
        return post("/collections/" + collectionName + "/points/scroll", json, ScrollResult.class);
    }

    // ==================== Utility Methods ====================

    /**
     * Checks server health.
     *
     * @return Health status
     */
    public HealthStatus health() throws LimyeDBException {
        return get("/health", HealthStatus.class);
    }

    /**
     * Gets server information.
     *
     * @return Server info as a map
     */
    public Map<String, Object> info() throws LimyeDBException {
        return get("/info", new TypeReference<Map<String, Object>>() {});
    }

    @Override
    public void close() {
        httpClient.dispatcher().executorService().shutdown();
        httpClient.connectionPool().evictAll();
    }

    // ==================== Private Helper Methods ====================

    private <T> T get(String path, Class<T> responseType) throws LimyeDBException {
        Request request = new Request.Builder()
                .url(baseUrl + path)
                .get()
                .build();

        return executeRequest(request, responseType);
    }

    private <T> T get(String path, TypeReference<T> typeReference) throws LimyeDBException {
        Request request = new Request.Builder()
                .url(baseUrl + path)
                .get()
                .build();

        return executeRequest(request, typeReference);
    }

    private <T> T post(String path, String json, Class<T> responseType) throws LimyeDBException {
        RequestBody body = RequestBody.create(json, JSON);
        Request request = new Request.Builder()
                .url(baseUrl + path)
                .post(body)
                .build();

        return executeRequest(request, responseType);
    }

    private <T> T put(String path, String json, Class<T> responseType) throws LimyeDBException {
        RequestBody body = RequestBody.create(json, JSON);
        Request request = new Request.Builder()
                .url(baseUrl + path)
                .put(body)
                .build();

        return executeRequest(request, responseType);
    }

    private void delete(String path) throws LimyeDBException {
        Request request = new Request.Builder()
                .url(baseUrl + path)
                .delete()
                .build();

        executeRequest(request, Void.class);
    }

    private <T> T executeRequest(Request request, Class<T> responseType) throws LimyeDBException {
        try (Response response = httpClient.newCall(request).execute()) {
            return handleResponse(response, responseType);
        } catch (IOException e) {
            throw new ConnectionException("Failed to connect to LimyeDB: " + e.getMessage(), e);
        }
    }

    private <T> T executeRequest(Request request, TypeReference<T> typeReference) throws LimyeDBException {
        try (Response response = httpClient.newCall(request).execute()) {
            return handleResponse(response, typeReference);
        } catch (IOException e) {
            throw new ConnectionException("Failed to connect to LimyeDB: " + e.getMessage(), e);
        }
    }

    private <T> T handleResponse(Response response, Class<T> responseType) throws LimyeDBException {
        int statusCode = response.code();
        String responseBody = "";

        try {
            ResponseBody body = response.body();
            if (body != null) {
                responseBody = body.string();
            }
        } catch (IOException e) {
            logger.warn("Failed to read response body: {}", e.getMessage());
        }

        if (statusCode == 401) {
            throw new AuthenticationException("Invalid API key");
        }

        if (statusCode == 404) {
            if (responseBody.toLowerCase().contains("collection")) {
                throw new CollectionNotFoundException("", responseBody);
            }
            throw new LimyeDBException("Not found: " + responseBody, 404);
        }

        if (!response.isSuccessful()) {
            throw new LimyeDBException("Request failed: " + responseBody, statusCode);
        }

        if (responseType == Void.class || responseBody.isEmpty()) {
            return null;
        }

        try {
            return objectMapper.readValue(responseBody, responseType);
        } catch (JsonProcessingException e) {
            throw new LimyeDBException("Failed to parse response: " + e.getMessage(), e);
        }
    }

    private <T> T handleResponse(Response response, TypeReference<T> typeReference) throws LimyeDBException {
        int statusCode = response.code();
        String responseBody = "";

        try {
            ResponseBody body = response.body();
            if (body != null) {
                responseBody = body.string();
            }
        } catch (IOException e) {
            logger.warn("Failed to read response body: {}", e.getMessage());
        }

        if (statusCode == 401) {
            throw new AuthenticationException("Invalid API key");
        }

        if (statusCode == 404) {
            if (responseBody.toLowerCase().contains("collection")) {
                throw new CollectionNotFoundException("", responseBody);
            }
            throw new LimyeDBException("Not found: " + responseBody, 404);
        }

        if (!response.isSuccessful()) {
            throw new LimyeDBException("Request failed: " + responseBody, statusCode);
        }

        if (responseBody.isEmpty()) {
            return null;
        }

        try {
            return objectMapper.readValue(responseBody, typeReference);
        } catch (JsonProcessingException e) {
            throw new LimyeDBException("Failed to parse response: " + e.getMessage(), e);
        }
    }

    private String toJson(Object obj) throws LimyeDBException {
        try {
            return objectMapper.writeValueAsString(obj);
        } catch (JsonProcessingException e) {
            throw new LimyeDBException("Failed to serialize request: " + e.getMessage(), e);
        }
    }

    // ==================== Response Types ====================

    /**
     * Collection information.
     */
    public static class CollectionInfo {
        private String name;
        private int dimension;
        private String metric;
        private long size;
        private String status;

        public String getName() { return name; }
        public int getDimension() { return dimension; }
        public String getMetric() { return metric; }
        public long getSize() { return size; }
        public String getStatus() { return status; }
    }

    /**
     * Upsert operation result.
     */
    public static class UpsertResult {
        private int succeeded;
        private int failed;

        public int getSucceeded() { return succeeded; }
        public int getFailed() { return failed; }
    }

    /**
     * Scroll result.
     */
    public static class ScrollResult {
        private List<Point> points;
        private String nextOffset;

        public List<Point> getPoints() { return points; }
        public String getNextOffset() { return nextOffset; }
    }

    /**
     * Health status.
     */
    public static class HealthStatus {
        private String status;
        private String version;

        public String getStatus() { return status; }
        public String getVersion() { return version; }
    }
}
