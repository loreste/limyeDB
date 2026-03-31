package hybrid

import (
	"sync"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/point"
)

func createRaceTestVectorIndex(t *testing.T, dim int) *hnsw.HNSW {
	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 100,
		EfSearch:       50,
		MaxElements:    1000,
		Metric:         config.MetricCosine,
		Dimension:      dim,
	}

	idx, err := hnsw.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create HNSW index: %v", err)
	}
	return idx
}

func TestRaceBM25IndexRemove(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	idx := NewBM25Index(nil)

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// Writers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				doc := &Document{
					ID:      string(rune('a'+id)) + string(rune('0'+i%10)),
					Content: "test content for document",
				}
				_ = idx.Index(doc)
			}
		}(g)
	}

	// Removers
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				docID := string(rune('a'+id)) + string(rune('0'+i%10))
				_ = idx.Remove(docID)
			}
		}(g)
	}

	// Searchers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = idx.Search("test", 10)
			}
		}()
	}

	wg.Wait()
}

func TestRaceBM25SearchWithBoost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	idx := NewBM25Index(nil)

	// Pre-populate
	for i := 0; i < 50; i++ {
		doc := &Document{
			ID:      string(rune('a' + i%26)),
			Content: "test content here",
			Fields:  map[string]string{"title": "title text"},
		}
		_ = idx.Index(doc)
	}

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				boosts := map[string]float64{"title": float64(i%3 + 1)}
				_ = idx.SearchWithBoost("test", 10, boosts)
			}
		}()
	}

	wg.Wait()
}

func TestRaceHybridSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	vectorIdx := createRaceTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	// Pre-populate
	for i := 0; i < 20; i++ {
		id := string(rune('a' + i%26))
		vec := make(point.Vector, 4)
		vec[i%4] = 1.0

		pt := point.NewPointWithID(id, vec, nil)
		_ = vectorIdx.Insert(pt)

		doc := &Document{ID: id, Content: "test content"}
		_ = textIdx.Index(doc)
	}

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	const goroutines = 10
	const ops = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				vec := make(point.Vector, 4)
				vec[i%4] = 1.0

				params := &HybridSearchParams{
					Vector: vec,
					Query:  "test",
					Limit:  5,
				}
				_, _ = searcher.Search(params)
			}
		}(g)
	}

	wg.Wait()
}

func TestRaceHybridUpdateWeights(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	vectorIdx := createRaceTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// Weight updaters
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				w := float64(i%10+1) / 10.0
				searcher.UpdateWeights(w, 1-w)
			}
		}(g)
	}

	// Method setters
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func() {
			defer wg.Done()
			methods := []FusionMethod{FusionRRF, FusionWeighted, FusionConvex}
			for i := 0; i < ops; i++ {
				searcher.SetFusionMethod(methods[i%len(methods)])
			}
		}()
	}

	wg.Wait()
}

func TestRaceHybridIndexText(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	vectorIdx := createRaceTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	const goroutines = 10
	const ops = 50

	var wg sync.WaitGroup

	// Indexers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				docID := string(rune('a'+id)) + string(rune('0'+i%10))
				_ = searcher.IndexText(docID, "content for document", nil)
			}
		}(g)
	}

	// Removers
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				docID := string(rune('a'+id)) + string(rune('0'+i%10))
				_ = searcher.RemoveText(docID)
			}
		}(g)
	}

	wg.Wait()
}

func TestRaceBM25Size(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	idx := NewBM25Index(nil)

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// Writers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				doc := &Document{
					ID:      string(rune('a'+id)) + string(rune('0'+i%10)),
					Content: "test",
				}
				_ = idx.Index(doc)
			}
		}(g)
	}

	// Size readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = idx.Size()
			}
		}()
	}

	wg.Wait()
}

func TestRaceBM25GetDocument(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	idx := NewBM25Index(nil)

	// Pre-populate
	for i := 0; i < 50; i++ {
		doc := &Document{
			ID:      string(rune('a' + i%26)),
			Content: "test content",
		}
		_ = idx.Index(doc)
	}

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// Readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				docID := string(rune('a' + i%26))
				_, _ = idx.GetDocument(docID)
			}
		}()
	}

	// Writers
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				doc := &Document{
					ID:      string(rune('a' + i%26)),
					Content: "updated content",
				}
				_ = idx.Index(doc)
			}
		}(g)
	}

	wg.Wait()
}
