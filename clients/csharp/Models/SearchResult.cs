using System.Text.Json.Serialization;

namespace LimyeDB.Models;

/// <summary>
/// Represents a search result.
/// </summary>
public class SearchResult
{
    /// <summary>
    /// Point ID.
    /// </summary>
    [JsonPropertyName("id")]
    public string Id { get; set; } = string.Empty;

    /// <summary>
    /// Similarity score.
    /// </summary>
    [JsonPropertyName("score")]
    public float Score { get; set; }

    /// <summary>
    /// Vector (if requested).
    /// </summary>
    [JsonPropertyName("vector")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public List<float>? Vector { get; set; }

    /// <summary>
    /// Payload (if requested).
    /// </summary>
    [JsonPropertyName("payload")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public Dictionary<string, object>? Payload { get; set; }

    /// <summary>
    /// Name of the vector used for search.
    /// </summary>
    [JsonPropertyName("vector_name")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? VectorName { get; set; }

    public override string ToString()
    {
        return $"SearchResult {{ Id = {Id}, Score = {Score} }}";
    }
}

/// <summary>
/// Response wrapper for search operations.
/// </summary>
internal class SearchResponse
{
    [JsonPropertyName("result")]
    public List<SearchResult>? Result { get; set; }

    [JsonPropertyName("took_ms")]
    public long TookMs { get; set; }
}

/// <summary>
/// Response wrapper for batch search operations.
/// </summary>
internal class BatchSearchResponse
{
    [JsonPropertyName("results")]
    public List<SearchResponse>? Results { get; set; }
}

/// <summary>
/// Response wrapper for hybrid search operations.
/// </summary>
internal class HybridSearchResponse
{
    [JsonPropertyName("results")]
    public List<SearchResult>? Results { get; set; }

    [JsonPropertyName("took_ms")]
    public long TookMs { get; set; }
}
