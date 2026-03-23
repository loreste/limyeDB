package sparse

import (
	"testing"
)

func TestSparseVectorDotProduct(t *testing.T) {
	v1 := NewVector([]uint32{1, 3, 5}, []float32{1.0, 2.0, 3.0})
	v2 := NewVector([]uint32{1, 3, 7}, []float32{4.0, 5.0, 6.0})

	// Dot product should be: 1*4 + 2*5 = 14
	dot := v1.DotProduct(v2)
	if dot != 14.0 {
		t.Errorf("Expected dot product 14.0, got %f", dot)
	}
}

func TestSparseVectorFromMap(t *testing.T) {
	m := map[uint32]float32{
		5: 3.0,
		1: 1.0,
		3: 2.0,
	}

	v := NewVectorFromMap(m)

	// Indices should be sorted
	if v.Indices[0] != 1 || v.Indices[1] != 3 || v.Indices[2] != 5 {
		t.Errorf("Indices not sorted: %v", v.Indices)
	}

	if v.Values[0] != 1.0 || v.Values[1] != 2.0 || v.Values[2] != 3.0 {
		t.Errorf("Values don't match indices: %v", v.Values)
	}
}

func TestSparseVectorEncodeDecode(t *testing.T) {
	original := NewVector([]uint32{1, 3, 5, 100}, []float32{1.0, 2.0, 3.0, 4.0})

	encoded := original.Encode()
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if len(decoded.Indices) != len(original.Indices) {
		t.Errorf("Length mismatch: expected %d, got %d", len(original.Indices), len(decoded.Indices))
	}

	for i := range original.Indices {
		if decoded.Indices[i] != original.Indices[i] {
			t.Errorf("Index mismatch at %d: expected %d, got %d", i, original.Indices[i], decoded.Indices[i])
		}
		if decoded.Values[i] != original.Values[i] {
			t.Errorf("Value mismatch at %d: expected %f, got %f", i, original.Values[i], decoded.Values[i])
		}
	}
}

func TestInvertedIndex(t *testing.T) {
	idx := NewInvertedIndex()

	// Add documents
	idx.Add(1, NewVector([]uint32{10, 20, 30}, []float32{1.0, 2.0, 3.0}))
	idx.Add(2, NewVector([]uint32{20, 30, 40}, []float32{1.5, 2.5, 3.5}))
	idx.Add(3, NewVector([]uint32{10, 40, 50}, []float32{1.0, 1.0, 1.0}))

	if idx.Size() != 3 {
		t.Errorf("Expected 3 documents, got %d", idx.Size())
	}

	// Search
	query := NewVector([]uint32{20, 30}, []float32{1.0, 1.0})
	results := idx.Search(query, 10)

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results, got %d", len(results))
	}

	// Doc 1 and 2 should both match (they have 20 and 30)
	foundDoc1, foundDoc2 := false, false
	for _, r := range results {
		if r.DocID == 1 {
			foundDoc1 = true
		}
		if r.DocID == 2 {
			foundDoc2 = true
		}
	}

	if !foundDoc1 || !foundDoc2 {
		t.Error("Expected to find doc 1 and doc 2 in results")
	}
}

func TestInvertedIndexRemove(t *testing.T) {
	idx := NewInvertedIndex()

	idx.Add(1, NewVector([]uint32{10, 20}, []float32{1.0, 1.0}))
	idx.Add(2, NewVector([]uint32{10, 30}, []float32{1.0, 1.0}))

	// Remove doc 1
	idx.Remove(1)

	// Search should only find doc 2
	query := NewVector([]uint32{10}, []float32{1.0})
	results := idx.Search(query, 10)

	if len(results) != 1 {
		t.Errorf("Expected 1 result after removal, got %d", len(results))
	}

	if len(results) > 0 && results[0].DocID != 2 {
		t.Errorf("Expected doc 2, got doc %d", results[0].DocID)
	}
}

func TestBM25Index(t *testing.T) {
	idx := NewBM25Index(DefaultBM25Config())

	// Add documents (simulating term frequencies)
	// Doc 1: "apple banana cherry"
	idx.Add(1, map[uint32]float32{100: 1, 200: 1, 300: 1})
	// Doc 2: "apple apple banana"
	idx.Add(2, map[uint32]float32{100: 2, 200: 1})
	// Doc 3: "cherry cherry cherry"
	idx.Add(3, map[uint32]float32{300: 3})

	// Search for "apple"
	results := idx.Search(map[uint32]float32{100: 1}, 10)

	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'apple', got %d", len(results))
	}

	// Doc 2 should rank higher (more apple occurrences)
	if len(results) >= 2 && results[0].DocID != 2 {
		t.Errorf("Expected doc 2 to rank first for 'apple', got doc %d", results[0].DocID)
	}
}

func TestHybridSearchRRF(t *testing.T) {
	denseResults := []ScoredDoc{
		{DocID: 1, Score: 0.9},
		{DocID: 2, Score: 0.8},
		{DocID: 3, Score: 0.7},
	}

	sparseResults := []ScoredDoc{
		{DocID: 2, Score: 0.95},
		{DocID: 4, Score: 0.85},
		{DocID: 1, Score: 0.75},
	}

	config := &HybridSearchConfig{
		Method:      FusionRRF,
		RRFConstant: 60,
	}

	results := FuseResults(denseResults, sparseResults, config, 5)

	if len(results) == 0 {
		t.Fatal("Expected results from RRF fusion")
	}

	// Doc 2 appears high in both lists, should rank highest
	if results[0].DocID != 2 && results[0].DocID != 1 {
		t.Logf("Top result: doc %d with score %f", results[0].DocID, results[0].FinalScore)
	}

	// Check that all docs are present
	docIDs := make(map[uint32]bool)
	for _, r := range results {
		docIDs[r.DocID] = true
	}

	if len(docIDs) != 4 {
		t.Errorf("Expected 4 unique docs (1,2,3,4), got %d", len(docIDs))
	}
}

func TestHybridSearchWeighted(t *testing.T) {
	denseResults := []ScoredDoc{
		{DocID: 1, Score: 0.9},
		{DocID: 2, Score: 0.5},
	}

	sparseResults := []ScoredDoc{
		{DocID: 2, Score: 1.0},
		{DocID: 1, Score: 0.3},
	}

	config := &HybridSearchConfig{
		Method:       FusionWeighted,
		DenseWeight:  0.3,
		SparseWeight: 0.7,
	}

	results := FuseResults(denseResults, sparseResults, config, 2)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// With sparse weight 0.7, doc 2 (high sparse score) should rank first
	if results[0].DocID != 2 {
		t.Errorf("Expected doc 2 to rank first with high sparse weight, got doc %d", results[0].DocID)
	}
}

func BenchmarkInvertedIndexSearch(b *testing.B) {
	idx := NewInvertedIndex()

	// Add 10000 documents
	for i := uint32(0); i < 10000; i++ {
		// Each doc has ~10 random terms
		indices := make([]uint32, 10)
		values := make([]float32, 10)
		for j := 0; j < 10; j++ {
			indices[j] = (i*7 + uint32(j*13)) % 1000 // Terms in range 0-999
			values[j] = 1.0
		}
		idx.Add(i, NewVector(indices, values))
	}

	query := NewVector([]uint32{100, 200, 300}, []float32{1.0, 1.0, 1.0})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search(query, 10)
	}
}

func BenchmarkBM25Search(b *testing.B) {
	idx := NewBM25Index(DefaultBM25Config())

	// Add 10000 documents
	for i := uint32(0); i < 10000; i++ {
		termFreqs := make(map[uint32]float32)
		for j := 0; j < 10; j++ {
			termID := (i*7 + uint32(j*13)) % 1000
			termFreqs[termID] = 1.0
		}
		idx.Add(i, termFreqs)
	}

	query := map[uint32]float32{100: 1.0, 200: 1.0, 300: 1.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search(query, 10)
	}
}
