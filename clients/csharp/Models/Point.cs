using System.Text.Json.Serialization;

namespace LimyeDB.Models;

/// <summary>
/// Represents a vector point in a collection.
/// </summary>
public class Point
{
    /// <summary>
    /// Unique identifier for the point.
    /// </summary>
    [JsonPropertyName("id")]
    public string Id { get; set; } = string.Empty;

    /// <summary>
    /// The vector embedding.
    /// </summary>
    [JsonPropertyName("vector")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public List<float>? Vector { get; set; }

    /// <summary>
    /// Optional payload (metadata).
    /// </summary>
    [JsonPropertyName("payload")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public Dictionary<string, object>? Payload { get; set; }

    /// <summary>
    /// Named vectors for multi-vector collections.
    /// </summary>
    [JsonPropertyName("named_vectors")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public Dictionary<string, List<float>>? NamedVectors { get; set; }

    /// <summary>
    /// Sparse vector representation.
    /// </summary>
    [JsonPropertyName("sparse")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public SparseVector? Sparse { get; set; }

    /// <summary>
    /// Creates a new point with the specified ID and vector.
    /// </summary>
    public static Point Create(string id, IEnumerable<float> vector)
    {
        return new Point
        {
            Id = id,
            Vector = vector.ToList()
        };
    }

    /// <summary>
    /// Creates a new point with the specified ID, vector, and payload.
    /// </summary>
    public static Point Create(string id, IEnumerable<float> vector, Dictionary<string, object> payload)
    {
        return new Point
        {
            Id = id,
            Vector = vector.ToList(),
            Payload = payload
        };
    }
}

/// <summary>
/// Represents a sparse vector with indices and values.
/// </summary>
public class SparseVector
{
    /// <summary>
    /// Non-zero indices.
    /// </summary>
    [JsonPropertyName("indices")]
    public List<int> Indices { get; set; } = new();

    /// <summary>
    /// Values at the corresponding indices.
    /// </summary>
    [JsonPropertyName("values")]
    public List<float> Values { get; set; } = new();

    /// <summary>
    /// Creates a new sparse vector.
    /// </summary>
    public static SparseVector Create(IEnumerable<int> indices, IEnumerable<float> values)
    {
        return new SparseVector
        {
            Indices = indices.ToList(),
            Values = values.ToList()
        };
    }
}
