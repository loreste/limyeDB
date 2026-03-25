package benchmark

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/quantization"
)

// toPointVectors converts [][]float32 to []point.Vector
func toPointVectors(vecs [][]float32) []point.Vector {
	result := make([]point.Vector, len(vecs))
	for i, v := range vecs {
		result[i] = point.Vector(v)
	}
	return result
}

// BenchmarkQuantizationEncode benchmarks quantization encoding speed
func BenchmarkQuantizationEncode(b *testing.B) {
	dimensions := []int{128, 384, 768, 1536}

	for _, dim := range dimensions {
		b.Run(fmt.Sprintf("Scalar_Dim%d", dim), func(b *testing.B) {
			q := quantization.NewScalarQuantizer(dim, 0.99)

			// Generate training data
			rng := rand.New(rand.NewSource(42))
			trainingData := make([][]float32, 1000)
			for i := range trainingData {
				trainingData[i] = make([]float32, dim)
				for j := range trainingData[i] {
					trainingData[i][j] = rng.Float32()*2 - 1
				}
			}

			// Train quantizer
			if err := q.Train(toPointVectors(trainingData)); err != nil {
				b.Fatalf("Failed to train quantizer: %v", err)
			}

			// Generate test vector
			testVec := make([]float32, dim)
			for j := range testVec {
				testVec[j] = rng.Float32()*2 - 1
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := q.Encode(testVec)
				if err != nil {
					b.Fatalf("Encode failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkQuantizationDecode benchmarks quantization decoding speed
func BenchmarkQuantizationDecode(b *testing.B) {
	dimensions := []int{128, 384, 768, 1536}

	for _, dim := range dimensions {
		b.Run(fmt.Sprintf("Scalar_Dim%d", dim), func(b *testing.B) {
			q := quantization.NewScalarQuantizer(dim, 0.99)

			rng := rand.New(rand.NewSource(42))
			trainingData := make([][]float32, 1000)
			for i := range trainingData {
				trainingData[i] = make([]float32, dim)
				for j := range trainingData[i] {
					trainingData[i][j] = rng.Float32()*2 - 1
				}
			}

			if err := q.Train(toPointVectors(trainingData)); err != nil {
				b.Fatalf("Failed to train quantizer: %v", err)
			}

			testVec := make([]float32, dim)
			for j := range testVec {
				testVec[j] = rng.Float32()*2 - 1
			}

			encoded, err := q.Encode(testVec)
			if err != nil {
				b.Fatalf("Encode failed: %v", err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := q.Decode(encoded)
				if err != nil {
					b.Fatalf("Decode failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkQuantizationDistance benchmarks quantized distance calculation
func BenchmarkQuantizationDistance(b *testing.B) {
	dimensions := []int{128, 384, 768, 1536}

	for _, dim := range dimensions {
		b.Run(fmt.Sprintf("Scalar_Dim%d", dim), func(b *testing.B) {
			q := quantization.NewScalarQuantizer(dim, 0.99)

			rng := rand.New(rand.NewSource(42))
			trainingData := make([][]float32, 1000)
			for i := range trainingData {
				trainingData[i] = make([]float32, dim)
				for j := range trainingData[i] {
					trainingData[i][j] = rng.Float32()*2 - 1
				}
			}

			if err := q.Train(toPointVectors(trainingData)); err != nil {
				b.Fatalf("Failed to train quantizer: %v", err)
			}

			queryVec := make([]float32, dim)
			docVec := make([]float32, dim)
			for j := range queryVec {
				queryVec[j] = rng.Float32()*2 - 1
				docVec[j] = rng.Float32()*2 - 1
			}

			encodedDoc, err := q.Encode(docVec)
			if err != nil {
				b.Fatalf("Encode failed: %v", err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = q.Distance(queryVec, encodedDoc)
			}
		})
	}
}

// BenchmarkQuantizedVsUnquantizedSearch compares search with and without quantization
func BenchmarkQuantizedVsUnquantizedSearch(b *testing.B) {
	numVectors := 50000
	dimension := 384
	k := 10
	ef := 100

	rng := rand.New(rand.NewSource(42))

	// Generate vectors
	vectors := make([][]float32, numVectors)
	for i := range vectors {
		vectors[i] = make([]float32, dimension)
		for j := range vectors[i] {
			vectors[i][j] = rng.Float32()*2 - 1
		}
	}

	query := make([]float32, dimension)
	for j := range query {
		query[j] = rng.Float32()*2 - 1
	}

	b.Run("Unquantized", func(b *testing.B) {
		cfg := &hnsw.Config{
			M:              16,
			EfConstruction: 200,
			EfSearch:       ef,
			MaxElements:    numVectors,
			Metric:         config.MetricCosine,
			Dimension:      dimension,
		}

		idx, err := hnsw.New(cfg)
		if err != nil {
			b.Fatalf("Failed to create index: %v", err)
		}

		for i, vec := range vectors {
			p := &point.Point{
				ID:     fmt.Sprintf("point-%d", i),
				Vector: vec,
			}
			if err := idx.Insert(p); err != nil {
				b.Fatalf("Failed to insert: %v", err)
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := idx.SearchWithEf(query, k, ef)
			if err != nil {
				b.Fatalf("Search failed: %v", err)
			}
		}
	})

	b.Run("ScalarQuantized", func(b *testing.B) {
		q := quantization.NewScalarQuantizer(dimension, 0.99)

		// Train quantizer
		trainingData := toPointVectors(vectors[:min(10000, len(vectors))])
		if err := q.Train(trainingData); err != nil {
			b.Fatalf("Failed to train quantizer: %v", err)
		}

		cfg := &hnsw.Config{
			M:              16,
			EfConstruction: 200,
			EfSearch:       ef,
			MaxElements:    numVectors,
			Metric:         config.MetricCosine,
			Dimension:      dimension,
			Quantizer:      q,
		}

		idx, err := hnsw.New(cfg)
		if err != nil {
			b.Fatalf("Failed to create index: %v", err)
		}

		for i, vec := range vectors {
			p := &point.Point{
				ID:     fmt.Sprintf("point-%d", i),
				Vector: vec,
			}
			if err := idx.Insert(p); err != nil {
				b.Fatalf("Failed to insert: %v", err)
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := idx.SearchWithEf(query, k, ef)
			if err != nil {
				b.Fatalf("Search failed: %v", err)
			}
		}
	})
}

// BenchmarkQuantizationMemory measures memory usage with quantization
func BenchmarkQuantizationMemory(b *testing.B) {
	numVectors := 10000
	dimension := 768

	rng := rand.New(rand.NewSource(42))

	vectors := make([][]float32, numVectors)
	for i := range vectors {
		vectors[i] = make([]float32, dimension)
		for j := range vectors[i] {
			vectors[i][j] = rng.Float32()*2 - 1
		}
	}

	b.Run("FullPrecision", func(b *testing.B) {
		// Full precision: 4 bytes per float
		expectedBytes := numVectors * dimension * 4
		b.ReportMetric(float64(expectedBytes)/(1024*1024), "MB_vectors")

		for i := 0; i < b.N; i++ {
			// Just measure allocation
			_ = make([][]float32, numVectors)
		}
	})

	b.Run("ScalarQuantized", func(b *testing.B) {
		q := quantization.NewScalarQuantizer(dimension, 0.99)

		trainingData := toPointVectors(vectors[:min(1000, len(vectors))])
		if err := q.Train(trainingData); err != nil {
			b.Fatalf("Failed to train: %v", err)
		}

		// Measure encoded size
		encoded, _ := q.Encode(vectors[0])
		bytesPerVector := len(encoded)
		expectedBytes := numVectors * bytesPerVector
		b.ReportMetric(float64(expectedBytes)/(1024*1024), "MB_vectors")
		b.ReportMetric(float64(numVectors*dimension*4)/float64(expectedBytes), "compression_ratio")

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = make([][]byte, numVectors)
		}
	})
}
