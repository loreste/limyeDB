using System.Text.Json.Serialization;

namespace LimyeDB.Models;

/// <summary>
/// Represents a filter for search queries.
/// </summary>
public class Filter
{
    /// <summary>
    /// All conditions must match.
    /// </summary>
    [JsonPropertyName("must")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public List<Condition>? Must { get; set; }

    /// <summary>
    /// None of the conditions must match.
    /// </summary>
    [JsonPropertyName("must_not")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public List<Condition>? MustNot { get; set; }

    /// <summary>
    /// At least one condition should match.
    /// </summary>
    [JsonPropertyName("should")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public List<Condition>? Should { get; set; }

    /// <summary>
    /// Adds a must condition.
    /// </summary>
    public Filter AddMust(Condition condition)
    {
        Must ??= new List<Condition>();
        Must.Add(condition);
        return this;
    }

    /// <summary>
    /// Adds a must_not condition.
    /// </summary>
    public Filter AddMustNot(Condition condition)
    {
        MustNot ??= new List<Condition>();
        MustNot.Add(condition);
        return this;
    }

    /// <summary>
    /// Adds a should condition.
    /// </summary>
    public Filter AddShould(Condition condition)
    {
        Should ??= new List<Condition>();
        Should.Add(condition);
        return this;
    }

    /// <summary>
    /// Creates a filter with a single must condition.
    /// </summary>
    public static Filter MustMatch(string key, object value)
    {
        return new Filter().AddMust(Condition.Match(key, value));
    }

    /// <summary>
    /// Creates a filter with a single range condition.
    /// </summary>
    public static Filter MustRange(string key, double? gt = null, double? gte = null, double? lt = null, double? lte = null)
    {
        return new Filter().AddMust(Condition.Range(key, gt, gte, lt, lte));
    }
}

/// <summary>
/// Represents a filter condition.
/// </summary>
public class Condition
{
    /// <summary>
    /// Field key.
    /// </summary>
    [JsonPropertyName("key")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? Key { get; set; }

    /// <summary>
    /// Match condition.
    /// </summary>
    [JsonPropertyName("match")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public MatchCondition? Match { get; set; }

    /// <summary>
    /// Range condition.
    /// </summary>
    [JsonPropertyName("range")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public RangeCondition? Range { get; set; }

    /// <summary>
    /// Creates a match condition.
    /// </summary>
    public static Condition Match(string key, object value)
    {
        return new Condition
        {
            Key = key,
            Match = new MatchCondition { Value = value }
        };
    }

    /// <summary>
    /// Creates a range condition.
    /// </summary>
    public static Condition Range(string key, double? gt = null, double? gte = null, double? lt = null, double? lte = null)
    {
        return new Condition
        {
            Key = key,
            Range = new RangeCondition
            {
                Gt = gt,
                Gte = gte,
                Lt = lt,
                Lte = lte
            }
        };
    }
}

/// <summary>
/// Match condition for exact value matching.
/// </summary>
public class MatchCondition
{
    /// <summary>
    /// Value to match.
    /// </summary>
    [JsonPropertyName("value")]
    public object? Value { get; set; }
}

/// <summary>
/// Range condition for numeric comparisons.
/// </summary>
public class RangeCondition
{
    /// <summary>
    /// Greater than.
    /// </summary>
    [JsonPropertyName("gt")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public double? Gt { get; set; }

    /// <summary>
    /// Greater than or equal.
    /// </summary>
    [JsonPropertyName("gte")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public double? Gte { get; set; }

    /// <summary>
    /// Less than.
    /// </summary>
    [JsonPropertyName("lt")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public double? Lt { get; set; }

    /// <summary>
    /// Less than or equal.
    /// </summary>
    [JsonPropertyName("lte")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public double? Lte { get; set; }

    /// <summary>
    /// Creates a greater than condition.
    /// </summary>
    public static RangeCondition GreaterThan(double value) => new() { Gt = value };

    /// <summary>
    /// Creates a greater than or equal condition.
    /// </summary>
    public static RangeCondition GreaterThanOrEqual(double value) => new() { Gte = value };

    /// <summary>
    /// Creates a less than condition.
    /// </summary>
    public static RangeCondition LessThan(double value) => new() { Lt = value };

    /// <summary>
    /// Creates a less than or equal condition.
    /// </summary>
    public static RangeCondition LessThanOrEqual(double value) => new() { Lte = value };

    /// <summary>
    /// Creates a between condition (inclusive).
    /// </summary>
    public static RangeCondition Between(double min, double max) => new() { Gte = min, Lte = max };
}
