package benchmark

import (
	"math/rand"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/distance"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/point"
)

const (
	dimension = 128
	numPoints = 10000
)

func generateRandomVector(dim int) point.Vector {
	v := make(point.Vector, dim)
	for i := range v {
		v[i] = rand.Float32()*2 - 1
	}
	return v
}

func generateRandomPoints(n, dim int) []*point.Point {
	points := make([]*point.Point, n)
	for i := range points {
		points[i] = point.NewPoint(generateRandomVector(dim), nil)
	}
	return points
}

// Distance Benchmarks

func BenchmarkCosineDistance(b *testing.B) {
	calc := &distance.Cosine{}
	v1 := generateRandomVector(dimension)
	v2 := generateRandomVector(dimension)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.Distance(v1, v2)
	}
}

func BenchmarkEuclideanDistance(b *testing.B) {
	calc := &distance.Euclidean{}
	v1 := generateRandomVector(dimension)
	v2 := generateRandomVector(dimension)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.Distance(v1, v2)
	}
}

func BenchmarkDotProductDistance(b *testing.B) {
	calc := &distance.DotProduct{}
	v1 := generateRandomVector(dimension)
	v2 := generateRandomVector(dimension)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.Distance(v1, v2)
	}
}

func BenchmarkBatchDistance(b *testing.B) {
	calc := &distance.Cosine{}
	query := generateRandomVector(dimension)
	vectors := make([]point.Vector, 1000)
	for i := range vectors {
		vectors[i] = generateRandomVector(dimension)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		distance.BatchDistance(calc, query, vectors)
	}
}

// HNSW Benchmarks

func BenchmarkHNSWInsert(b *testing.B) {
	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       100,
		MaxElements:    b.N + 1000,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	index, _ := hnsw.New(cfg)
	points := generateRandomPoints(b.N, dimension)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index.Insert(points[i])
	}
}

func BenchmarkHNSWSearch(b *testing.B) {
	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       100,
		MaxElements:    numPoints,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	index, _ := hnsw.New(cfg)
	points := generateRandomPoints(numPoints, dimension)

	for _, p := range points {
		index.Insert(p)
	}

	query := generateRandomVector(dimension)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index.Search(query, 10)
	}
}

func BenchmarkHNSWSearchK100(b *testing.B) {
	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       200,
		MaxElements:    numPoints,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	index, _ := hnsw.New(cfg)
	points := generateRandomPoints(numPoints, dimension)

	for _, p := range points {
		index.Insert(p)
	}

	query := generateRandomVector(dimension)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index.Search(query, 100)
	}
}

func BenchmarkHNSWBatchSearch(b *testing.B) {
	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       100,
		MaxElements:    numPoints,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	index, _ := hnsw.New(cfg)
	points := generateRandomPoints(numPoints, dimension)

	for _, p := range points {
		index.Insert(p)
	}

	queries := make([]point.Vector, 100)
	for i := range queries {
		queries[i] = generateRandomVector(dimension)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index.BatchSearch(queries, 10)
	}
}

// Point Benchmarks

func BenchmarkPointEncode(b *testing.B) {
	p := point.NewPoint(generateRandomVector(dimension), map[string]interface{}{
		"category": "test",
		"score":    0.95,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf [4096]byte
		writer := &byteWriter{buf: buf[:0]}
		p.Encode(writer)
	}
}

func BenchmarkVectorNormalize(b *testing.B) {
	v := generateRandomVector(dimension)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v2 := make(point.Vector, len(v))
		copy(v2, v)
		v2.Normalize()
	}
}

// Memory efficiency benchmarks

func BenchmarkMemoryEfficiency(b *testing.B) {
	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 100,
		EfSearch:       50,
		MaxElements:    100000,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	index, _ := hnsw.New(cfg)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p := point.NewPoint(generateRandomVector(dimension), nil)
		index.Insert(p)
	}
}

// Parallel benchmarks

func BenchmarkParallelSearch(b *testing.B) {
	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       100,
		MaxElements:    numPoints,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	index, _ := hnsw.New(cfg)
	points := generateRandomPoints(numPoints, dimension)

	for _, p := range points {
		index.Insert(p)
	}

	b.RunParallel(func(pb *testing.PB) {
		query := generateRandomVector(dimension)
		for pb.Next() {
			index.Search(query, 10)
		}
	})
}

// Different dimension benchmarks

func BenchmarkSearch_Dim64(b *testing.B)   { benchmarkSearchWithDim(b, 64) }
func BenchmarkSearch_Dim128(b *testing.B)  { benchmarkSearchWithDim(b, 128) }
func BenchmarkSearch_Dim256(b *testing.B)  { benchmarkSearchWithDim(b, 256) }
func BenchmarkSearch_Dim512(b *testing.B)  { benchmarkSearchWithDim(b, 512) }
func BenchmarkSearch_Dim1024(b *testing.B) { benchmarkSearchWithDim(b, 1024) }

func benchmarkSearchWithDim(b *testing.B, dim int) {
	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       100,
		MaxElements:    5000,
		Metric:         config.MetricCosine,
		Dimension:      dim,
	}

	index, _ := hnsw.New(cfg)
	points := generateRandomPoints(5000, dim)

	for _, p := range points {
		index.Insert(p)
	}

	query := generateRandomVector(dim)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index.Search(query, 10)
	}
}

// Helper types

type byteWriter struct {
	buf []byte
}

func (w *byteWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}
