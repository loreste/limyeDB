package com.limyedb;

import com.limyedb.exceptions.*;
import com.limyedb.models.*;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.*;

import java.io.IOException;
import java.util.*;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Unit tests for LimyeDBClient.
 */
class LimyeDBClientTest {

    private MockWebServer mockServer;
    private LimyeDBClient client;

    @BeforeEach
    void setUp() throws IOException {
        mockServer = new MockWebServer();
        mockServer.start();
        client = new LimyeDBClient(mockServer.url("/").toString(), "test-api-key");
    }

    @AfterEach
    void tearDown() throws IOException {
        client.close();
        mockServer.shutdown();
    }

    @Test
    void testCreateCollection() throws Exception {
        String responseJson = "{\"name\":\"test\",\"dimension\":128,\"metric\":\"cosine\",\"size\":0,\"status\":\"ready\"}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        CollectionConfig config = CollectionConfig.builder()
                .name("test")
                .dimension(128)
                .metric("cosine")
                .build();

        LimyeDBClient.CollectionInfo result = client.createCollection(config);

        assertNotNull(result);
        assertEquals("test", result.getName());
        assertEquals(128, result.getDimension());
        assertEquals("cosine", result.getMetric());

        RecordedRequest request = mockServer.takeRequest();
        assertEquals("POST", request.getMethod());
        assertEquals("/collections", request.getPath());
        assertTrue(request.getHeader("Authorization").contains("Bearer test-api-key"));
    }

    @Test
    void testGetCollection() throws Exception {
        String responseJson = "{\"name\":\"test\",\"dimension\":128,\"metric\":\"cosine\",\"size\":100,\"status\":\"ready\"}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        LimyeDBClient.CollectionInfo result = client.getCollection("test");

        assertNotNull(result);
        assertEquals("test", result.getName());
        assertEquals(100, result.getSize());

        RecordedRequest request = mockServer.takeRequest();
        assertEquals("GET", request.getMethod());
        assertEquals("/collections/test", request.getPath());
    }

    @Test
    void testListCollections() throws Exception {
        String responseJson = "{\"collections\":[{\"name\":\"test1\",\"dimension\":128},{\"name\":\"test2\",\"dimension\":256}]}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        List<LimyeDBClient.CollectionInfo> result = client.listCollections();

        assertNotNull(result);
        assertEquals(2, result.size());
        assertEquals("test1", result.get(0).getName());
        assertEquals("test2", result.get(1).getName());
    }

    @Test
    void testDeleteCollection() throws Exception {
        mockServer.enqueue(new MockResponse()
                .setResponseCode(200)
                .addHeader("Content-Type", "application/json"));

        assertDoesNotThrow(() -> client.deleteCollection("test"));

        RecordedRequest request = mockServer.takeRequest();
        assertEquals("DELETE", request.getMethod());
        assertEquals("/collections/test", request.getPath());
    }

    @Test
    void testCollectionExists() throws Exception {
        // Exists
        mockServer.enqueue(new MockResponse()
                .setBody("{\"name\":\"test\"}")
                .addHeader("Content-Type", "application/json"));

        assertTrue(client.collectionExists("test"));

        // Does not exist
        mockServer.enqueue(new MockResponse()
                .setResponseCode(404)
                .setBody("collection not found")
                .addHeader("Content-Type", "application/json"));

        assertFalse(client.collectionExists("nonexistent"));
    }

    @Test
    void testUpsert() throws Exception {
        String responseJson = "{\"succeeded\":2,\"failed\":0}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        List<Point> points = Arrays.asList(
                Point.builder()
                        .id("point1")
                        .vector(Arrays.asList(0.1f, 0.2f, 0.3f, 0.4f))
                        .payload(Map.of("name", "test1"))
                        .build(),
                Point.builder()
                        .id("point2")
                        .vector(Arrays.asList(0.5f, 0.6f, 0.7f, 0.8f))
                        .payload(Map.of("name", "test2"))
                        .build()
        );

        LimyeDBClient.UpsertResult result = client.upsert("test", points);

        assertNotNull(result);
        assertEquals(2, result.getSucceeded());
        assertEquals(0, result.getFailed());

        RecordedRequest request = mockServer.takeRequest();
        assertEquals("PUT", request.getMethod());
        assertEquals("/collections/test/points", request.getPath());
    }

    @Test
    void testGetPoint() throws Exception {
        String responseJson = "{\"id\":\"point1\",\"vector\":[0.1,0.2,0.3,0.4],\"payload\":{\"name\":\"test\"}}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        Point result = client.getPoint("test", "point1");

        assertNotNull(result);
        assertEquals("point1", result.getId());
        assertNotNull(result.getVector());
        assertEquals(4, result.getVector().size());

        RecordedRequest request = mockServer.takeRequest();
        assertEquals("GET", request.getMethod());
        assertTrue(request.getPath().startsWith("/collections/test/points/point1"));
    }

    @Test
    void testDeletePoints() throws Exception {
        String responseJson = "{\"deleted\":2}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        int result = client.deletePoints("test", Arrays.asList("point1", "point2"));

        assertEquals(2, result);

        RecordedRequest request = mockServer.takeRequest();
        assertEquals("POST", request.getMethod());
        assertEquals("/collections/test/points/delete", request.getPath());
    }

    @Test
    void testSearch() throws Exception {
        String responseJson = "{\"result\":[{\"id\":\"point1\",\"score\":0.95,\"payload\":{\"name\":\"test1\"}},{\"id\":\"point2\",\"score\":0.85,\"payload\":{\"name\":\"test2\"}}]}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        List<Float> query = Arrays.asList(0.1f, 0.2f, 0.3f, 0.4f);
        List<SearchResult> results = client.search("test", query, 10);

        assertNotNull(results);
        assertEquals(2, results.size());
        assertEquals("point1", results.get(0).getId());
        assertEquals(0.95f, results.get(0).getScore(), 0.001);

        RecordedRequest request = mockServer.takeRequest();
        assertEquals("POST", request.getMethod());
        assertEquals("/collections/test/search", request.getPath());
    }

    @Test
    void testSearchWithFilter() throws Exception {
        String responseJson = "{\"result\":[{\"id\":\"point1\",\"score\":0.95}]}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        List<Float> query = Arrays.asList(0.1f, 0.2f, 0.3f, 0.4f);
        Filter filter = Filter.builder()
                .addMust(Filter.Condition.match("category", "A"))
                .build();

        List<SearchResult> results = client.search("test", query, 10, filter);

        assertNotNull(results);
        assertEquals(1, results.size());

        RecordedRequest request = mockServer.takeRequest();
        String body = request.getBody().readUtf8();
        assertTrue(body.contains("filter"));
        assertTrue(body.contains("category"));
    }

    @Test
    void testSearchBatch() throws Exception {
        String responseJson = "{\"results\":[{\"result\":[{\"id\":\"point1\",\"score\":0.9}]},{\"result\":[{\"id\":\"point2\",\"score\":0.8}]}]}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        List<List<Float>> queries = Arrays.asList(
                Arrays.asList(0.1f, 0.2f, 0.3f, 0.4f),
                Arrays.asList(0.5f, 0.6f, 0.7f, 0.8f)
        );

        List<List<SearchResult>> results = client.searchBatch("test", queries, 10);

        assertNotNull(results);
        assertEquals(2, results.size());
        assertEquals(1, results.get(0).size());
        assertEquals(1, results.get(1).size());
    }

    @Test
    void testScroll() throws Exception {
        String responseJson = "{\"points\":[{\"id\":\"point1\"},{\"id\":\"point2\"}],\"nextOffset\":\"point2\"}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        LimyeDBClient.ScrollResult result = client.scroll("test", 10);

        assertNotNull(result);
        assertNotNull(result.getPoints());
        assertEquals(2, result.getPoints().size());
        assertEquals("point2", result.getNextOffset());
    }

    @Test
    void testHealth() throws Exception {
        String responseJson = "{\"status\":\"ok\",\"version\":\"1.0.0\"}";
        mockServer.enqueue(new MockResponse()
                .setBody(responseJson)
                .addHeader("Content-Type", "application/json"));

        LimyeDBClient.HealthStatus result = client.health();

        assertNotNull(result);
        assertEquals("ok", result.getStatus());
        assertEquals("1.0.0", result.getVersion());
    }

    @Test
    void testAuthenticationError() {
        mockServer.enqueue(new MockResponse()
                .setResponseCode(401)
                .setBody("Unauthorized"));

        assertThrows(AuthenticationException.class, () -> client.getCollection("test"));
    }

    @Test
    void testCollectionNotFoundError() {
        mockServer.enqueue(new MockResponse()
                .setResponseCode(404)
                .setBody("Collection not found"));

        assertThrows(CollectionNotFoundException.class, () -> client.getCollection("nonexistent"));
    }

    @Test
    void testPointBuilder() {
        Point point = Point.builder()
                .id("test-id")
                .vector(0.1f, 0.2f, 0.3f)
                .payload(Map.of("key", "value"))
                .build();

        assertEquals("test-id", point.getId());
        assertEquals(3, point.getVector().size());
        assertEquals(0.1f, point.getVector().get(0), 0.001);
        assertEquals("value", point.getPayload().get("key"));
    }

    @Test
    void testFilterBuilder() {
        Filter filter = Filter.builder()
                .addMust(Filter.Condition.match("category", "A"))
                .addMust(Filter.Condition.range("price", null, 10.0, 100.0, null))
                .addMustNot(Filter.Condition.match("status", "deleted"))
                .build();

        assertNotNull(filter.getMust());
        assertEquals(2, filter.getMust().size());
        assertNotNull(filter.getMustNot());
        assertEquals(1, filter.getMustNot().size());
    }

    @Test
    void testHNSWConfigBuilder() {
        HNSWConfig config = HNSWConfig.builder()
                .m(16)
                .efConstruction(200)
                .efSearch(100)
                .maxElements(100000)
                .build();

        assertEquals(16, config.getM());
        assertEquals(200, config.getEfConstruction());
        assertEquals(100, config.getEfSearch());
        assertEquals(100000, config.getMaxElements());
    }

    @Test
    void testCollectionConfigBuilder() {
        CollectionConfig config = CollectionConfig.builder()
                .name("test")
                .dimension(128)
                .metric("cosine")
                .hnsw(HNSWConfig.builder().m(16).build())
                .onDisk(true)
                .shardCount(3)
                .replicationFactor(2)
                .build();

        assertEquals("test", config.getName());
        assertEquals(128, config.getDimension());
        assertEquals("cosine", config.getMetric());
        assertNotNull(config.getHnsw());
        assertTrue(config.getOnDisk());
        assertEquals(3, config.getShardCount());
        assertEquals(2, config.getReplicationFactor());
    }
}
