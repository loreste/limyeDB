using System.Net;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using LimyeDB.Exceptions;
using LimyeDB.Models;

namespace LimyeDB;

/// <summary>
/// Official .NET Client for LimyeDB - Enterprise Distributed Vector Database.
/// </summary>
/// <example>
/// <code>
/// using var client = new LimyeDBClient("http://localhost:8080", "your-api-key");
///
/// // Create a collection
/// await client.CreateCollectionAsync(new CollectionConfig
/// {
///     Name = "documents",
///     Dimension = 1536,
///     Metric = "cosine"
/// });
///
/// // Insert vectors
/// await client.UpsertAsync("documents", new[]
/// {
///     Point.Create("doc1", new[] { 0.1f, 0.2f, ... },
///         new Dictionary&lt;string, object&gt; { { "title", "Hello" } })
/// });
///
/// // Search
/// var results = await client.SearchAsync("documents", new[] { 0.1f, 0.2f, ... }, 10);
/// </code>
/// </example>
public class LimyeDBClient : IDisposable
{
    private readonly HttpClient _httpClient;
    private readonly JsonSerializerOptions _jsonOptions;
    private bool _disposed;

    /// <summary>
    /// Creates a new LimyeDB client.
    /// </summary>
    /// <param name="host">Server URL (e.g., "http://localhost:8080")</param>
    /// <param name="apiKey">API key for authentication (can be null)</param>
    /// <param name="timeout">Request timeout (default: 30 seconds)</param>
    public LimyeDBClient(string host, string? apiKey = null, TimeSpan? timeout = null)
    {
        var baseUrl = host.TrimEnd('/');

        _httpClient = new HttpClient
        {
            BaseAddress = new Uri(baseUrl),
            Timeout = timeout ?? TimeSpan.FromSeconds(30)
        };

        _httpClient.DefaultRequestHeaders.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        if (!string.IsNullOrEmpty(apiKey))
        {
            _httpClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", apiKey);
        }

        _jsonOptions = new JsonSerializerOptions
        {
            PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
            DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull
        };
    }

    #region Collection Operations

    /// <summary>
    /// Creates a new collection.
    /// </summary>
    public async Task<CollectionInfo> CreateCollectionAsync(CollectionConfig config, CancellationToken cancellationToken = default)
    {
        return await PostAsync<CollectionInfo>("/collections", config, cancellationToken);
    }

    /// <summary>
    /// Gets information about a collection.
    /// </summary>
    public async Task<CollectionInfo> GetCollectionAsync(string name, CancellationToken cancellationToken = default)
    {
        return await GetAsync<CollectionInfo>($"/collections/{name}", cancellationToken);
    }

    /// <summary>
    /// Lists all collections.
    /// </summary>
    public async Task<List<CollectionInfo>> ListCollectionsAsync(CancellationToken cancellationToken = default)
    {
        var response = await GetAsync<CollectionsResponse>("/collections", cancellationToken);
        return response.Collections ?? new List<CollectionInfo>();
    }

    /// <summary>
    /// Deletes a collection.
    /// </summary>
    public async Task DeleteCollectionAsync(string name, CancellationToken cancellationToken = default)
    {
        await DeleteAsync($"/collections/{name}", cancellationToken);
    }

    /// <summary>
    /// Checks if a collection exists.
    /// </summary>
    public async Task<bool> CollectionExistsAsync(string name, CancellationToken cancellationToken = default)
    {
        try
        {
            await GetCollectionAsync(name, cancellationToken);
            return true;
        }
        catch (CollectionNotFoundException)
        {
            return false;
        }
    }

    #endregion

    #region Point Operations

    /// <summary>
    /// Upserts points into a collection.
    /// </summary>
    public async Task<UpsertResult> UpsertAsync(string collectionName, IEnumerable<Point> points, bool wait = true, CancellationToken cancellationToken = default)
    {
        var payload = new { points = points.ToList(), wait };
        return await PutAsync<UpsertResult>($"/collections/{collectionName}/points", payload, cancellationToken);
    }

    /// <summary>
    /// Upserts points in batches.
    /// </summary>
    public async Task<List<UpsertResult>> UpsertBatchAsync(string collectionName, IEnumerable<Point> points, int batchSize = 100, CancellationToken cancellationToken = default)
    {
        var results = new List<UpsertResult>();
        var pointsList = points.ToList();

        for (int i = 0; i < pointsList.Count; i += batchSize)
        {
            var batch = pointsList.Skip(i).Take(batchSize);
            var result = await UpsertAsync(collectionName, batch, true, cancellationToken);
            results.Add(result);
        }

        return results;
    }

    /// <summary>
    /// Gets a single point by ID.
    /// </summary>
    public async Task<Point> GetPointAsync(string collectionName, string pointId, bool withVector = true, bool withPayload = true, CancellationToken cancellationToken = default)
    {
        var url = $"/collections/{collectionName}/points/{pointId}?with_vector={withVector}&with_payload={withPayload}";
        return await GetAsync<Point>(url, cancellationToken);
    }

    /// <summary>
    /// Gets multiple points by IDs.
    /// </summary>
    public async Task<List<Point>> GetPointsAsync(string collectionName, IEnumerable<string> ids, bool withVector = true, bool withPayload = true, CancellationToken cancellationToken = default)
    {
        var payload = new { ids = ids.ToList(), with_vector = withVector, with_payload = withPayload };
        var response = await PostAsync<PointsResponse>($"/collections/{collectionName}/points/get", payload, cancellationToken);
        return response.Points ?? new List<Point>();
    }

    /// <summary>
    /// Deletes points by IDs.
    /// </summary>
    public async Task<int> DeletePointsAsync(string collectionName, IEnumerable<string> ids, CancellationToken cancellationToken = default)
    {
        var payload = new { ids = ids.ToList() };
        var response = await PostAsync<DeleteResponse>($"/collections/{collectionName}/points/delete", payload, cancellationToken);
        return response.Deleted;
    }

    #endregion

    #region Search Operations

    /// <summary>
    /// Searches for similar vectors.
    /// </summary>
    public async Task<List<SearchResult>> SearchAsync(string collectionName, IEnumerable<float> vector, int limit, CancellationToken cancellationToken = default)
    {
        return await SearchAsync(collectionName, vector, limit, null, 100, true, false, cancellationToken);
    }

    /// <summary>
    /// Searches for similar vectors with filter.
    /// </summary>
    public async Task<List<SearchResult>> SearchAsync(string collectionName, IEnumerable<float> vector, int limit, Filter? filter, CancellationToken cancellationToken = default)
    {
        return await SearchAsync(collectionName, vector, limit, filter, 100, true, false, cancellationToken);
    }

    /// <summary>
    /// Searches for similar vectors with full options.
    /// </summary>
    public async Task<List<SearchResult>> SearchAsync(
        string collectionName,
        IEnumerable<float> vector,
        int limit,
        Filter? filter,
        int ef,
        bool withPayload,
        bool withVector,
        CancellationToken cancellationToken = default)
    {
        var payload = new Dictionary<string, object>
        {
            { "vector", vector.ToList() },
            { "limit", limit },
            { "ef", ef },
            { "with_payload", withPayload },
            { "with_vector", withVector }
        };

        if (filter != null)
        {
            payload["filter"] = filter;
        }

        var response = await PostAsync<SearchResponse>($"/collections/{collectionName}/search", payload, cancellationToken);
        return response.Result ?? new List<SearchResult>();
    }

    /// <summary>
    /// Performs batch search for multiple queries.
    /// </summary>
    public async Task<List<List<SearchResult>>> SearchBatchAsync(
        string collectionName,
        IEnumerable<IEnumerable<float>> vectors,
        int limit,
        Filter? filter = null,
        bool withPayload = true,
        CancellationToken cancellationToken = default)
    {
        var searches = vectors.Select(v =>
        {
            var search = new Dictionary<string, object>
            {
                { "vector", v.ToList() },
                { "limit", limit },
                { "with_payload", withPayload }
            };
            if (filter != null)
            {
                search["filter"] = filter;
            }
            return search;
        }).ToList();

        var payload = new { searches };
        var response = await PostAsync<BatchSearchResponse>($"/collections/{collectionName}/search/batch", payload, cancellationToken);

        return response.Results?.Select(r => r.Result ?? new List<SearchResult>()).ToList() ?? new List<List<SearchResult>>();
    }

    /// <summary>
    /// Performs hybrid search combining dense and sparse vectors.
    /// </summary>
    public async Task<List<SearchResult>> HybridSearchAsync(
        string collectionName,
        IEnumerable<float>? denseVector,
        SparseVector? sparseQuery,
        int limit,
        Filter? filter = null,
        string fusionMethod = "rrf",
        int fusionK = 60,
        bool withPayload = true,
        CancellationToken cancellationToken = default)
    {
        var payload = new Dictionary<string, object>
        {
            { "limit", limit },
            { "with_payload", withPayload },
            { "fusion", new { method = fusionMethod, k = fusionK } }
        };

        if (denseVector != null)
        {
            payload["dense_vector"] = denseVector.ToList();
        }

        if (sparseQuery != null)
        {
            payload["sparse_query"] = sparseQuery;
        }

        if (filter != null)
        {
            payload["filter"] = filter;
        }

        var response = await PostAsync<HybridSearchResponse>($"/collections/{collectionName}/search/hybrid", payload, cancellationToken);
        return response.Results ?? new List<SearchResult>();
    }

    /// <summary>
    /// Scrolls through points in a collection.
    /// </summary>
    public async Task<ScrollResult> ScrollAsync(
        string collectionName,
        int limit = 100,
        string? offset = null,
        Filter? filter = null,
        bool withPayload = true,
        bool withVector = false,
        CancellationToken cancellationToken = default)
    {
        var payload = new Dictionary<string, object>
        {
            { "limit", limit },
            { "with_payload", withPayload },
            { "with_vector", withVector }
        };

        if (offset != null)
        {
            payload["offset"] = offset;
        }

        if (filter != null)
        {
            payload["filter"] = filter;
        }

        return await PostAsync<ScrollResult>($"/collections/{collectionName}/points/scroll", payload, cancellationToken);
    }

    #endregion

    #region Utility Methods

    /// <summary>
    /// Checks server health.
    /// </summary>
    public async Task<HealthStatus> HealthAsync(CancellationToken cancellationToken = default)
    {
        return await GetAsync<HealthStatus>("/health", cancellationToken);
    }

    /// <summary>
    /// Gets server information.
    /// </summary>
    public async Task<Dictionary<string, object>> InfoAsync(CancellationToken cancellationToken = default)
    {
        return await GetAsync<Dictionary<string, object>>("/info", cancellationToken);
    }

    #endregion

    #region HTTP Methods

    private async Task<T> GetAsync<T>(string path, CancellationToken cancellationToken)
    {
        var response = await _httpClient.GetAsync(path, cancellationToken);
        return await HandleResponseAsync<T>(response, cancellationToken);
    }

    private async Task<T> PostAsync<T>(string path, object payload, CancellationToken cancellationToken)
    {
        var json = JsonSerializer.Serialize(payload, _jsonOptions);
        var content = new StringContent(json, Encoding.UTF8, "application/json");
        var response = await _httpClient.PostAsync(path, content, cancellationToken);
        return await HandleResponseAsync<T>(response, cancellationToken);
    }

    private async Task<T> PutAsync<T>(string path, object payload, CancellationToken cancellationToken)
    {
        var json = JsonSerializer.Serialize(payload, _jsonOptions);
        var content = new StringContent(json, Encoding.UTF8, "application/json");
        var response = await _httpClient.PutAsync(path, content, cancellationToken);
        return await HandleResponseAsync<T>(response, cancellationToken);
    }

    private async Task DeleteAsync(string path, CancellationToken cancellationToken)
    {
        var response = await _httpClient.DeleteAsync(path, cancellationToken);
        await HandleResponseAsync<object>(response, cancellationToken);
    }

    private async Task<T> HandleResponseAsync<T>(HttpResponseMessage response, CancellationToken cancellationToken)
    {
        var content = await response.Content.ReadAsStringAsync(cancellationToken);

        if (response.StatusCode == HttpStatusCode.Unauthorized)
        {
            throw new AuthenticationException("Invalid API key");
        }

        if (response.StatusCode == HttpStatusCode.NotFound)
        {
            if (content.Contains("collection", StringComparison.OrdinalIgnoreCase))
            {
                throw new CollectionNotFoundException("", content);
            }
            throw new LimyeDBException($"Not found: {content}", 404);
        }

        if (!response.IsSuccessStatusCode)
        {
            throw new LimyeDBException($"Request failed: {content}", (int)response.StatusCode);
        }

        if (string.IsNullOrEmpty(content) || typeof(T) == typeof(object))
        {
            return default!;
        }

        try
        {
            return JsonSerializer.Deserialize<T>(content, _jsonOptions)!;
        }
        catch (JsonException ex)
        {
            throw new LimyeDBException($"Failed to parse response: {ex.Message}", ex);
        }
    }

    #endregion

    #region Response Types

    private class CollectionsResponse
    {
        [JsonPropertyName("collections")]
        public List<CollectionInfo>? Collections { get; set; }
    }

    private class PointsResponse
    {
        [JsonPropertyName("points")]
        public List<Point>? Points { get; set; }
    }

    private class DeleteResponse
    {
        [JsonPropertyName("deleted")]
        public int Deleted { get; set; }
    }

    #endregion

    #region IDisposable

    public void Dispose()
    {
        Dispose(true);
        GC.SuppressFinalize(this);
    }

    protected virtual void Dispose(bool disposing)
    {
        if (!_disposed)
        {
            if (disposing)
            {
                _httpClient.Dispose();
            }
            _disposed = true;
        }
    }

    #endregion
}

/// <summary>
/// Upsert operation result.
/// </summary>
public class UpsertResult
{
    [JsonPropertyName("succeeded")]
    public int Succeeded { get; set; }

    [JsonPropertyName("failed")]
    public int Failed { get; set; }
}

/// <summary>
/// Scroll result.
/// </summary>
public class ScrollResult
{
    [JsonPropertyName("points")]
    public List<Point>? Points { get; set; }

    [JsonPropertyName("next_offset")]
    public string? NextOffset { get; set; }
}

/// <summary>
/// Health status.
/// </summary>
public class HealthStatus
{
    [JsonPropertyName("status")]
    public string Status { get; set; } = string.Empty;

    [JsonPropertyName("version")]
    public string Version { get; set; } = string.Empty;
}
