using System.Text.Json.Serialization;

namespace LimyeDB.Models;

/// <summary>
/// Configuration for creating a new collection.
/// </summary>
public class CollectionConfig
{
    /// <summary>
    /// Collection name.
    /// </summary>
    [JsonPropertyName("name")]
    public string Name { get; set; } = string.Empty;

    /// <summary>
    /// Vector dimension.
    /// </summary>
    [JsonPropertyName("dimension")]
    public int Dimension { get; set; }

    /// <summary>
    /// Distance metric (cosine, euclidean, dot_product).
    /// </summary>
    [JsonPropertyName("metric")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? Metric { get; set; }

    /// <summary>
    /// HNSW index configuration.
    /// </summary>
    [JsonPropertyName("hnsw")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public HNSWConfig? Hnsw { get; set; }

    /// <summary>
    /// Whether to store vectors on disk.
    /// </summary>
    [JsonPropertyName("on_disk")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public bool? OnDisk { get; set; }

    /// <summary>
    /// Named vector configurations.
    /// </summary>
    [JsonPropertyName("vectors")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public Dictionary<string, VectorConfig>? Vectors { get; set; }

    /// <summary>
    /// Number of shards.
    /// </summary>
    [JsonPropertyName("shard_count")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public int? ShardCount { get; set; }

    /// <summary>
    /// Replication factor.
    /// </summary>
    [JsonPropertyName("replication_factor")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public int? ReplicationFactor { get; set; }
}

/// <summary>
/// Configuration for a named vector.
/// </summary>
public class VectorConfig
{
    /// <summary>
    /// Vector dimension.
    /// </summary>
    [JsonPropertyName("dimension")]
    public int Dimension { get; set; }

    /// <summary>
    /// Distance metric.
    /// </summary>
    [JsonPropertyName("metric")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? Metric { get; set; }

    /// <summary>
    /// HNSW configuration.
    /// </summary>
    [JsonPropertyName("hnsw")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public HNSWConfig? Hnsw { get; set; }

    /// <summary>
    /// Whether to store on disk.
    /// </summary>
    [JsonPropertyName("on_disk")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public bool? OnDisk { get; set; }
}

/// <summary>
/// Collection information returned by the server.
/// </summary>
public class CollectionInfo
{
    [JsonPropertyName("name")]
    public string Name { get; set; } = string.Empty;

    [JsonPropertyName("dimension")]
    public int Dimension { get; set; }

    [JsonPropertyName("metric")]
    public string Metric { get; set; } = string.Empty;

    [JsonPropertyName("size")]
    public long Size { get; set; }

    [JsonPropertyName("status")]
    public string Status { get; set; } = string.Empty;
}
