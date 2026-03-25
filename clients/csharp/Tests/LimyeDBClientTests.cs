using LimyeDB.Exceptions;
using LimyeDB.Models;
using Xunit;

namespace LimyeDB.Tests;

/// <summary>
/// Unit tests for LimyeDBClient.
/// </summary>
public class LimyeDBClientTests
{
    [Fact]
    public void PointCreate_ShouldCreatePointWithIdAndVector()
    {
        var vector = new[] { 0.1f, 0.2f, 0.3f, 0.4f };
        var point = Point.Create("test-id", vector);

        Assert.Equal("test-id", point.Id);
        Assert.NotNull(point.Vector);
        Assert.Equal(4, point.Vector.Count);
        Assert.Equal(0.1f, point.Vector[0], 3);
    }

    [Fact]
    public void PointCreate_ShouldCreatePointWithPayload()
    {
        var vector = new[] { 0.1f, 0.2f, 0.3f };
        var payload = new Dictionary<string, object> { { "name", "test" }, { "count", 42 } };
        var point = Point.Create("test-id", vector, payload);

        Assert.Equal("test-id", point.Id);
        Assert.NotNull(point.Payload);
        Assert.Equal("test", point.Payload["name"]);
        Assert.Equal(42, point.Payload["count"]);
    }

    [Fact]
    public void SparseVectorCreate_ShouldCreateWithIndicesAndValues()
    {
        var indices = new[] { 0, 5, 10 };
        var values = new[] { 0.1f, 0.5f, 1.0f };
        var sparse = SparseVector.Create(indices, values);

        Assert.Equal(3, sparse.Indices.Count);
        Assert.Equal(3, sparse.Values.Count);
        Assert.Equal(5, sparse.Indices[1]);
        Assert.Equal(0.5f, sparse.Values[1], 3);
    }

    [Fact]
    public void FilterMustMatch_ShouldCreateFilterWithMustCondition()
    {
        var filter = Filter.MustMatch("category", "A");

        Assert.NotNull(filter.Must);
        Assert.Single(filter.Must);
        Assert.Equal("category", filter.Must[0].Key);
        Assert.NotNull(filter.Must[0].Match);
        Assert.Equal("A", filter.Must[0].Match.Value);
    }

    [Fact]
    public void FilterMustRange_ShouldCreateFilterWithRangeCondition()
    {
        var filter = Filter.MustRange("price", gte: 10, lte: 100);

        Assert.NotNull(filter.Must);
        Assert.Single(filter.Must);
        Assert.Equal("price", filter.Must[0].Key);
        Assert.NotNull(filter.Must[0].Range);
        Assert.Equal(10, filter.Must[0].Range.Gte);
        Assert.Equal(100, filter.Must[0].Range.Lte);
    }

    [Fact]
    public void FilterBuilder_ShouldBuildComplexFilter()
    {
        var filter = new Filter()
            .AddMust(Condition.Match("category", "A"))
            .AddMust(Condition.Range("price", gte: 10, lte: 100))
            .AddMustNot(Condition.Match("status", "deleted"))
            .AddShould(Condition.Match("featured", true));

        Assert.NotNull(filter.Must);
        Assert.Equal(2, filter.Must.Count);
        Assert.NotNull(filter.MustNot);
        Assert.Single(filter.MustNot);
        Assert.NotNull(filter.Should);
        Assert.Single(filter.Should);
    }

    [Fact]
    public void RangeCondition_GreaterThan_ShouldSetGtOnly()
    {
        var range = RangeCondition.GreaterThan(10);

        Assert.Equal(10, range.Gt);
        Assert.Null(range.Gte);
        Assert.Null(range.Lt);
        Assert.Null(range.Lte);
    }

    [Fact]
    public void RangeCondition_Between_ShouldSetGteAndLte()
    {
        var range = RangeCondition.Between(10, 100);

        Assert.Null(range.Gt);
        Assert.Equal(10, range.Gte);
        Assert.Null(range.Lt);
        Assert.Equal(100, range.Lte);
    }

    [Fact]
    public void HNSWConfigDefault_ShouldReturnDefaultValues()
    {
        var config = HNSWConfig.Default();

        Assert.Equal(16, config.M);
        Assert.Equal(200, config.EfConstruction);
        Assert.Equal(100, config.EfSearch);
    }

    [Fact]
    public void CollectionConfig_ShouldSetAllProperties()
    {
        var config = new CollectionConfig
        {
            Name = "test",
            Dimension = 128,
            Metric = "cosine",
            Hnsw = HNSWConfig.Default(),
            OnDisk = true,
            ShardCount = 3,
            ReplicationFactor = 2
        };

        Assert.Equal("test", config.Name);
        Assert.Equal(128, config.Dimension);
        Assert.Equal("cosine", config.Metric);
        Assert.NotNull(config.Hnsw);
        Assert.True(config.OnDisk);
        Assert.Equal(3, config.ShardCount);
        Assert.Equal(2, config.ReplicationFactor);
    }

    [Fact]
    public void VectorConfig_ShouldSetAllProperties()
    {
        var config = new VectorConfig
        {
            Dimension = 256,
            Metric = "euclidean",
            Hnsw = new HNSWConfig { M = 32 },
            OnDisk = false
        };

        Assert.Equal(256, config.Dimension);
        Assert.Equal("euclidean", config.Metric);
        Assert.NotNull(config.Hnsw);
        Assert.Equal(32, config.Hnsw.M);
        Assert.False(config.OnDisk);
    }

    [Fact]
    public void LimyeDBException_ShouldStoreStatusCode()
    {
        var exception = new LimyeDBException("Test error", 500);

        Assert.Equal("Test error", exception.Message);
        Assert.Equal(500, exception.StatusCode);
    }

    [Fact]
    public void AuthenticationException_ShouldHave401StatusCode()
    {
        var exception = new AuthenticationException("Invalid API key");

        Assert.Equal(401, exception.StatusCode);
    }

    [Fact]
    public void CollectionNotFoundException_ShouldStoreCollectionName()
    {
        var exception = new CollectionNotFoundException("test-collection");

        Assert.Equal("test-collection", exception.CollectionName);
        Assert.Equal(404, exception.StatusCode);
    }

    [Fact]
    public void SearchResult_ToString_ShouldReturnFormattedString()
    {
        var result = new SearchResult
        {
            Id = "point1",
            Score = 0.95f
        };

        var str = result.ToString();

        Assert.Contains("point1", str);
        Assert.Contains("0.95", str);
    }

    [Fact]
    public void ConditionMatch_ShouldCreateCorrectCondition()
    {
        var condition = Condition.Match("field", "value");

        Assert.Equal("field", condition.Key);
        Assert.NotNull(condition.Match);
        Assert.Equal("value", condition.Match.Value);
        Assert.Null(condition.Range);
    }

    [Fact]
    public void ConditionRange_ShouldCreateCorrectCondition()
    {
        var condition = Condition.Range("field", gt: 10, lt: 100);

        Assert.Equal("field", condition.Key);
        Assert.NotNull(condition.Range);
        Assert.Equal(10, condition.Range.Gt);
        Assert.Equal(100, condition.Range.Lt);
        Assert.Null(condition.Match);
    }

    [Fact]
    public void Client_ShouldDisposeWithoutError()
    {
        var client = new LimyeDBClient("http://localhost:8080");
        client.Dispose();

        // Disposing twice should not throw
        client.Dispose();
    }

    [Fact]
    public void CollectionConfigWithNamedVectors_ShouldSetVectors()
    {
        var config = new CollectionConfig
        {
            Name = "multi-vector",
            Vectors = new Dictionary<string, VectorConfig>
            {
                { "text", new VectorConfig { Dimension = 1536, Metric = "cosine" } },
                { "image", new VectorConfig { Dimension = 512, Metric = "euclidean" } }
            }
        };

        Assert.NotNull(config.Vectors);
        Assert.Equal(2, config.Vectors.Count);
        Assert.Equal(1536, config.Vectors["text"].Dimension);
        Assert.Equal(512, config.Vectors["image"].Dimension);
    }

    [Fact]
    public void PointWithNamedVectors_ShouldSetMultipleVectors()
    {
        var point = new Point
        {
            Id = "multi-1",
            NamedVectors = new Dictionary<string, List<float>>
            {
                { "text", new List<float> { 0.1f, 0.2f } },
                { "image", new List<float> { 0.3f, 0.4f } }
            }
        };

        Assert.NotNull(point.NamedVectors);
        Assert.Equal(2, point.NamedVectors.Count);
        Assert.Equal(2, point.NamedVectors["text"].Count);
        Assert.Equal(2, point.NamedVectors["image"].Count);
    }
}
