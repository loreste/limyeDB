using System.Text.Json.Serialization;

namespace LimyeDB.Models;

/// <summary>
/// HNSW index configuration.
/// </summary>
public class HNSWConfig
{
    /// <summary>
    /// Maximum number of connections per node.
    /// </summary>
    [JsonPropertyName("m")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public int? M { get; set; }

    /// <summary>
    /// Build-time quality parameter.
    /// </summary>
    [JsonPropertyName("ef_construction")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public int? EfConstruction { get; set; }

    /// <summary>
    /// Search-time quality parameter.
    /// </summary>
    [JsonPropertyName("ef_search")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public int? EfSearch { get; set; }

    /// <summary>
    /// Maximum number of elements.
    /// </summary>
    [JsonPropertyName("max_elements")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public int? MaxElements { get; set; }

    /// <summary>
    /// Creates a new HNSWConfig with default values.
    /// </summary>
    public static HNSWConfig Default() => new()
    {
        M = 16,
        EfConstruction = 200,
        EfSearch = 100
    };
}
