package geo

import (
	"math"
	"testing"
)

func TestHaversineDistance(t *testing.T) {
	// New York to Los Angeles: ~3940 km
	nyc := Point{Lat: 40.7128, Lon: -74.0060}
	la := Point{Lat: 34.0522, Lon: -118.2437}

	dist := HaversineDistance(nyc, la)
	expected := 3940.0

	if math.Abs(dist-expected) > 50 { // Allow 50km tolerance
		t.Errorf("Expected distance ~%f km, got %f km", expected, dist)
	}
}

func TestBoundingBoxContains(t *testing.T) {
	// Box covering NYC area
	bb := BoundingBox{
		TopLeft:     Point{Lat: 41.0, Lon: -74.5},
		BottomRight: Point{Lat: 40.5, Lon: -73.5},
	}

	// NYC should be inside
	nyc := Point{Lat: 40.7128, Lon: -74.0060}
	if !bb.Contains(nyc) {
		t.Error("NYC should be inside bounding box")
	}

	// LA should be outside
	la := Point{Lat: 34.0522, Lon: -118.2437}
	if bb.Contains(la) {
		t.Error("LA should be outside bounding box")
	}
}

func TestPolygonContains(t *testing.T) {
	// Simple triangle
	triangle := Polygon{
		Exterior: []Point{
			{Lat: 0, Lon: 0},
			{Lat: 10, Lon: 5},
			{Lat: 0, Lon: 10},
		},
	}

	// Point inside
	inside := Point{Lat: 3, Lon: 5}
	if !triangle.Contains(inside) {
		t.Error("Point should be inside triangle")
	}

	// Point outside
	outside := Point{Lat: 15, Lon: 5}
	if triangle.Contains(outside) {
		t.Error("Point should be outside triangle")
	}
}

func TestPolygonWithHole(t *testing.T) {
	// Square with a smaller square hole
	poly := Polygon{
		Exterior: []Point{
			{Lat: 0, Lon: 0},
			{Lat: 10, Lon: 0},
			{Lat: 10, Lon: 10},
			{Lat: 0, Lon: 10},
		},
		Interior: [][]Point{
			{
				{Lat: 3, Lon: 3},
				{Lat: 7, Lon: 3},
				{Lat: 7, Lon: 7},
				{Lat: 3, Lon: 7},
			},
		},
	}

	// Point in outer but not in hole
	inOuter := Point{Lat: 1, Lon: 1}
	if !poly.Contains(inOuter) {
		t.Error("Point in outer ring should be contained")
	}

	// Point in hole
	inHole := Point{Lat: 5, Lon: 5}
	if poly.Contains(inHole) {
		t.Error("Point in hole should not be contained")
	}
}

func TestRadiusFilter(t *testing.T) {
	center := Point{Lat: 40.7128, Lon: -74.0060} // NYC
	filter := NewRadiusFilter(center, 10000)     // 10km radius

	// Point 5km away should match
	nearby := Point{Lat: 40.75, Lon: -74.0}
	if !filter.Match(nearby) {
		t.Error("Nearby point should match radius filter")
	}

	// Point 100km away should not match
	faraway := Point{Lat: 41.5, Lon: -74.0}
	if filter.Match(faraway) {
		t.Error("Far point should not match radius filter")
	}
}

func TestGeoIndex(t *testing.T) {
	idx := NewIndex()

	// Add some points
	idx.Add(1, Point{Lat: 40.7128, Lon: -74.0060})  // NYC
	idx.Add(2, Point{Lat: 40.7580, Lon: -73.9855})  // Central Park
	idx.Add(3, Point{Lat: 34.0522, Lon: -118.2437}) // LA
	idx.Add(4, Point{Lat: 40.6892, Lon: -74.0445})  // Statue of Liberty

	if idx.Size() != 4 {
		t.Errorf("Expected 4 points, got %d", idx.Size())
	}

	// Search radius around NYC (20km)
	results := idx.SearchRadius(Point{Lat: 40.7128, Lon: -74.0060}, 20000, 10)

	// Should find NYC, Central Park, Statue of Liberty but not LA
	if len(results) != 3 {
		t.Errorf("Expected 3 results within 20km of NYC, got %d", len(results))
	}

	// First result should be NYC itself (closest)
	if results[0].DocID != 1 {
		t.Errorf("Expected doc 1 (NYC) as closest, got %d", results[0].DocID)
	}
}

func TestGeoIndexBoundingBox(t *testing.T) {
	idx := NewIndex()

	idx.Add(1, Point{Lat: 40.7128, Lon: -74.0060})  // NYC
	idx.Add(2, Point{Lat: 34.0522, Lon: -118.2437}) // LA
	idx.Add(3, Point{Lat: 41.8781, Lon: -87.6298})  // Chicago

	// Bounding box covering east coast
	bb := BoundingBox{
		TopLeft:     Point{Lat: 45.0, Lon: -80.0},
		BottomRight: Point{Lat: 35.0, Lon: -70.0},
	}

	results := idx.SearchBoundingBox(bb, 10)

	// Should only find NYC
	if len(results) != 1 {
		t.Errorf("Expected 1 result in east coast box, got %d", len(results))
	}

	if len(results) > 0 && results[0].DocID != 1 {
		t.Errorf("Expected doc 1 (NYC), got %d", results[0].DocID)
	}
}

func TestGeoIndexRemove(t *testing.T) {
	idx := NewIndex()

	idx.Add(1, Point{Lat: 40.7128, Lon: -74.0060})
	idx.Add(2, Point{Lat: 40.7580, Lon: -73.9855})

	idx.Remove(1)

	if idx.Size() != 1 {
		t.Errorf("Expected 1 point after removal, got %d", idx.Size())
	}

	// Search should only find doc 2
	results := idx.SearchRadius(Point{Lat: 40.7128, Lon: -74.0060}, 50000, 10)
	if len(results) != 1 || results[0].DocID != 2 {
		t.Error("After removal, only doc 2 should be found")
	}
}

func TestGeohashEncode(t *testing.T) {
	// Test geohash encoding produces consistent results
	nyc := Point{Lat: 40.7128, Lon: -74.0060}
	hash := Encode(nyc, 6)

	if len(hash) != 6 {
		t.Errorf("Expected 6 character geohash, got %d", len(hash))
	}

	// Same point should produce same hash
	hash2 := Encode(nyc, 6)
	if hash != hash2 {
		t.Errorf("Same point should produce same hash: %s vs %s", hash, hash2)
	}

	// Different points should produce different hashes
	la := Point{Lat: 34.0522, Lon: -118.2437}
	hashLA := Encode(la, 6)
	if hash == hashLA {
		t.Error("Different points should produce different hashes")
	}
}

func TestSearchFilter(t *testing.T) {
	idx := NewIndex()

	idx.Add(1, Point{Lat: 40.7128, Lon: -74.0060})
	idx.Add(2, Point{Lat: 34.0522, Lon: -118.2437})

	// Test radius filter
	radiusFilter := NewRadiusFilter(Point{Lat: 40.7128, Lon: -74.0060}, 10000)
	results := idx.SearchFilter(radiusFilter, 10)
	if len(results) != 1 || results[0].DocID != 1 {
		t.Error("Radius filter should find only NYC")
	}

	// Test bounding box filter
	bb := NewBoundingBox(Point{Lat: 35.0, Lon: -120.0}, Point{Lat: 33.0, Lon: -115.0})
	bbFilter := NewBoundingBoxFilter(bb)
	results = idx.SearchFilter(bbFilter, 10)
	if len(results) != 1 || results[0].DocID != 2 {
		t.Error("Bounding box filter should find only LA")
	}
}

func BenchmarkGeoIndexSearchRadius(b *testing.B) {
	idx := NewIndex()

	// Add 10000 random points
	for i := uint32(0); i < 10000; i++ {
		lat := -90.0 + float64(i%180)
		lon := -180.0 + float64(i%360)
		idx.Add(i, Point{Lat: lat, Lon: lon})
	}

	center := Point{Lat: 40.0, Lon: -74.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.SearchRadius(center, 100000, 10)
	}
}

func BenchmarkHaversineDistance(b *testing.B) {
	p1 := Point{Lat: 40.7128, Lon: -74.0060}
	p2 := Point{Lat: 34.0522, Lon: -118.2437}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HaversineDistance(p1, p2)
	}
}
