package quantization

import (
	"math"
	"math/rand"
	"testing"

	"github.com/limyedb/limyedb/pkg/point"
)

func TestScalarQuantizer(t *testing.T) {
	dim := 128
	q := NewScalarQuantizer(dim, 0.99)

	// Generate training vectors
	vectors := make([]point.Vector, 1000)
	for i := range vectors {
		vectors[i] = make(point.Vector, dim)
		for j := range vectors[i] {
			vectors[i][j] = rand.Float32()*2 - 1 // -1 to 1
		}
	}

	// Train
	err := q.Train(vectors)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	// Test encode/decode
	testVec := vectors[0]
	encoded, err := q.Encode(testVec)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(encoded) != dim {
		t.Errorf("Expected %d bytes, got %d", dim, len(encoded))
	}

	decoded, err := q.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Check reconstruction error is reasonable
	var mse float32
	for i := range testVec {
		diff := testVec[i] - decoded[i]
		mse += diff * diff
	}
	mse /= float32(dim)

	if mse > 0.01 {
		t.Errorf("Reconstruction error too high: MSE = %f", mse)
	}

	// Test compression ratio
	if q.CompressionRatio() != 4.0 {
		t.Errorf("Expected compression ratio 4.0, got %f", q.CompressionRatio())
	}
}

func TestBinaryQuantizer(t *testing.T) {
	dim := 128
	q := NewBinaryQuantizer(dim)

	// Generate training vectors
	vectors := make([]point.Vector, 1000)
	for i := range vectors {
		vectors[i] = make(point.Vector, dim)
		for j := range vectors[i] {
			vectors[i][j] = rand.Float32()*2 - 1
		}
	}

	// Train
	err := q.Train(vectors)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	// Test encode
	testVec := vectors[0]
	encoded, err := q.Encode(testVec)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expectedSize := (dim + 7) / 8
	if len(encoded) != expectedSize {
		t.Errorf("Expected %d bytes, got %d", expectedSize, len(encoded))
	}

	// Test compression ratio
	if q.CompressionRatio() != 32.0 {
		t.Errorf("Expected compression ratio 32.0, got %f", q.CompressionRatio())
	}

	// Test distance preserves relative ordering
	// Similar vectors should have smaller hamming distance
	similarVec := make(point.Vector, dim)
	copy(similarVec, testVec)
	for i := 0; i < 10; i++ {
		similarVec[i] += 0.1 // Small perturbation
	}

	differentVec := make(point.Vector, dim)
	for i := range differentVec {
		differentVec[i] = -testVec[i] // Opposite direction
	}

	similarEncoded, _ := q.Encode(similarVec)
	differentEncoded, _ := q.Encode(differentVec)

	distSimilar := q.Distance(testVec, similarEncoded)
	distDifferent := q.Distance(testVec, differentEncoded)

	if distSimilar >= distDifferent {
		t.Errorf("Similar vector should have smaller distance: similar=%f, different=%f",
			distSimilar, distDifferent)
	}
}

func TestProductQuantizer(t *testing.T) {
	dim := 128
	segments := 8
	centroids := 256

	q := NewProductQuantizer(dim, segments, centroids)

	// Generate training vectors
	vectors := make([]point.Vector, 1000)
	for i := range vectors {
		vectors[i] = make(point.Vector, dim)
		for j := range vectors[i] {
			vectors[i][j] = rand.Float32()*2 - 1
		}
	}

	// Train
	err := q.Train(vectors)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	// Test encode/decode
	testVec := vectors[0]
	encoded, err := q.Encode(testVec)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(encoded) != segments {
		t.Errorf("Expected %d bytes, got %d", segments, len(encoded))
	}

	decoded, err := q.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if len(decoded) != dim {
		t.Errorf("Expected dimension %d, got %d", dim, len(decoded))
	}

	// Compression ratio should be dimension*4 / segments
	expectedRatio := float32(dim*4) / float32(segments)
	if math.Abs(float64(q.CompressionRatio()-expectedRatio)) > 0.1 {
		t.Errorf("Expected compression ratio ~%f, got %f", expectedRatio, q.CompressionRatio())
	}
}

func TestBatchDistance(t *testing.T) {
	dim := 128
	q := NewScalarQuantizer(dim, 0.99)

	// Generate training vectors
	vectors := make([]point.Vector, 100)
	for i := range vectors {
		vectors[i] = make(point.Vector, dim)
		for j := range vectors[i] {
			vectors[i][j] = rand.Float32()*2 - 1
		}
	}

	q.Train(vectors)

	// Encode all vectors
	encoded := make([][]byte, len(vectors))
	for i, v := range vectors {
		encoded[i], _ = q.Encode(v)
	}

	// Test batch distance
	query := vectors[0]
	distances := q.BatchDistance(query, encoded)

	if len(distances) != len(vectors) {
		t.Errorf("Expected %d distances, got %d", len(vectors), len(distances))
	}

	// First distance should be very small (self)
	if distances[0] > 0.01 {
		t.Errorf("Self-distance should be near 0, got %f", distances[0])
	}
}

func BenchmarkScalarEncode(b *testing.B) {
	dim := 128
	q := NewScalarQuantizer(dim, 0.99)

	vectors := make([]point.Vector, 1000)
	for i := range vectors {
		vectors[i] = make(point.Vector, dim)
		for j := range vectors[i] {
			vectors[i][j] = rand.Float32()*2 - 1
		}
	}
	q.Train(vectors)

	testVec := vectors[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Encode(testVec)
	}
}

func BenchmarkBinaryEncode(b *testing.B) {
	dim := 128
	q := NewBinaryQuantizer(dim)

	vectors := make([]point.Vector, 1000)
	for i := range vectors {
		vectors[i] = make(point.Vector, dim)
		for j := range vectors[i] {
			vectors[i][j] = rand.Float32()*2 - 1
		}
	}
	q.Train(vectors)

	testVec := vectors[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Encode(testVec)
	}
}

func BenchmarkBinaryDistance(b *testing.B) {
	dim := 1024 // Higher dimension to show popcount benefits
	q := NewBinaryQuantizer(dim)

	vectors := make([]point.Vector, 1000)
	for i := range vectors {
		vectors[i] = make(point.Vector, dim)
		for j := range vectors[i] {
			vectors[i][j] = rand.Float32()*2 - 1
		}
	}
	q.Train(vectors)

	query := vectors[0]
	encoded, _ := q.Encode(vectors[1])

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Distance(query, encoded)
	}
}

func BenchmarkPQBatchDistance(b *testing.B) {
	dim := 128
	q := NewProductQuantizer(dim, 8, 256)

	vectors := make([]point.Vector, 1000)
	for i := range vectors {
		vectors[i] = make(point.Vector, dim)
		for j := range vectors[i] {
			vectors[i][j] = rand.Float32()*2 - 1
		}
	}
	q.Train(vectors)

	encoded := make([][]byte, len(vectors))
	for i, v := range vectors {
		encoded[i], _ = q.Encode(v)
	}

	query := vectors[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.BatchDistance(query, encoded)
	}
}
