package geo

import (
	"encoding/json"
	"math"
	"sort"
	"sync"
)

// Point represents a geographic point
type Point struct {
	Lat float64 `json:"lat"` // Latitude in degrees (-90 to 90)
	Lon float64 `json:"lon"` // Longitude in degrees (-180 to 180)
}

// NewPoint creates a new geo point
func NewPoint(lat, lon float64) Point {
	return Point{Lat: lat, Lon: lon}
}

// Valid checks if the point has valid coordinates
func (p Point) Valid() bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180
}

// BoundingBox represents a geographic bounding box
type BoundingBox struct {
	TopLeft     Point `json:"top_left"`
	BottomRight Point `json:"bottom_right"`
}

// NewBoundingBox creates a new bounding box
func NewBoundingBox(topLeft, bottomRight Point) BoundingBox {
	return BoundingBox{TopLeft: topLeft, BottomRight: bottomRight}
}

// Contains checks if a point is within the bounding box
func (bb BoundingBox) Contains(p Point) bool {
	// Handle normal case
	if bb.TopLeft.Lon <= bb.BottomRight.Lon {
		return p.Lat <= bb.TopLeft.Lat &&
			p.Lat >= bb.BottomRight.Lat &&
			p.Lon >= bb.TopLeft.Lon &&
			p.Lon <= bb.BottomRight.Lon
	}

	// Handle case where box crosses antimeridian (180/-180)
	return p.Lat <= bb.TopLeft.Lat &&
		p.Lat >= bb.BottomRight.Lat &&
		(p.Lon >= bb.TopLeft.Lon || p.Lon <= bb.BottomRight.Lon)
}

// Polygon represents a geographic polygon
type Polygon struct {
	Exterior []Point   `json:"exterior"` // Outer boundary (clockwise)
	Interior [][]Point `json:"interior"` // Holes (counter-clockwise)
}

// NewPolygon creates a new polygon from exterior points
func NewPolygon(exterior []Point) Polygon {
	return Polygon{Exterior: exterior}
}

// Contains checks if a point is within the polygon using ray casting
func (poly Polygon) Contains(p Point) bool {
	// Check exterior
	if !pointInRing(p, poly.Exterior) {
		return false
	}

	// Check holes
	for _, hole := range poly.Interior {
		if pointInRing(p, hole) {
			return false
		}
	}

	return true
}

// pointInRing checks if a point is inside a ring using ray casting algorithm
func pointInRing(p Point, ring []Point) bool {
	if len(ring) < 3 {
		return false
	}

	inside := false
	j := len(ring) - 1

	for i := 0; i < len(ring); i++ {
		if ((ring[i].Lat > p.Lat) != (ring[j].Lat > p.Lat)) &&
			(p.Lon < (ring[j].Lon-ring[i].Lon)*(p.Lat-ring[i].Lat)/(ring[j].Lat-ring[i].Lat)+ring[i].Lon) {
			inside = !inside
		}
		j = i
	}

	return inside
}

// =============================================================================
// Distance calculations
// =============================================================================

const earthRadiusKm = 6371.0 // Earth's radius in kilometers

// HaversineDistance calculates the distance between two points in kilometers
func HaversineDistance(p1, p2 Point) float64 {
	lat1 := toRadians(p1.Lat)
	lat2 := toRadians(p2.Lat)
	deltaLat := toRadians(p2.Lat - p1.Lat)
	deltaLon := toRadians(p2.Lon - p1.Lon)

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

// DistanceMeters returns distance in meters
func DistanceMeters(p1, p2 Point) float64 {
	return HaversineDistance(p1, p2) * 1000
}

func toRadians(deg float64) float64 {
	return deg * math.Pi / 180
}

// =============================================================================
// Geo Filters
// =============================================================================

// FilterType represents the type of geo filter
type FilterType string

const (
	FilterRadius      FilterType = "geo_radius"
	FilterBoundingBox FilterType = "geo_bounding_box"
	FilterPolygon     FilterType = "geo_polygon"
)

// Filter represents a geographic filter
type Filter struct {
	Type FilterType `json:"type"`

	// For radius filter
	Center Point   `json:"center,omitempty"`
	Radius float64 `json:"radius,omitempty"` // Radius in meters

	// For bounding box filter
	BoundingBox *BoundingBox `json:"bounding_box,omitempty"`

	// For polygon filter
	Polygon *Polygon `json:"polygon,omitempty"`
}

// NewRadiusFilter creates a radius filter
func NewRadiusFilter(center Point, radiusMeters float64) *Filter {
	return &Filter{
		Type:   FilterRadius,
		Center: center,
		Radius: radiusMeters,
	}
}

// NewBoundingBoxFilter creates a bounding box filter
func NewBoundingBoxFilter(bb BoundingBox) *Filter {
	return &Filter{
		Type:        FilterBoundingBox,
		BoundingBox: &bb,
	}
}

// NewPolygonFilter creates a polygon filter
func NewPolygonFilter(poly Polygon) *Filter {
	return &Filter{
		Type:    FilterPolygon,
		Polygon: &poly,
	}
}

// Match checks if a point matches the filter
func (f *Filter) Match(p Point) bool {
	switch f.Type {
	case FilterRadius:
		return DistanceMeters(f.Center, p) <= f.Radius
	case FilterBoundingBox:
		if f.BoundingBox == nil {
			return false
		}
		return f.BoundingBox.Contains(p)
	case FilterPolygon:
		if f.Polygon == nil {
			return false
		}
		return f.Polygon.Contains(p)
	default:
		return false
	}
}

// =============================================================================
// Geo Index using S2-like grid cells
// =============================================================================

// Index is a spatial index for efficient geo queries
type Index struct {
	// Grid-based index (simple approach)
	// Key: grid cell ID, Value: list of (docID, point) pairs
	cells map[uint64][]geoEntry

	// All points for iteration
	points map[uint32]Point

	// Grid parameters
	cellSize float64 // Cell size in degrees

	mu sync.RWMutex
}

type geoEntry struct {
	DocID uint32
	Point Point
}

// NewIndex creates a new geo index
func NewIndex() *Index {
	return &Index{
		cells:    make(map[uint64][]geoEntry),
		points:   make(map[uint32]Point),
		cellSize: 0.1, // ~11km at equator
	}
}

// NewIndexWithCellSize creates a geo index with custom cell size
func NewIndexWithCellSize(cellSizeDegrees float64) *Index {
	return &Index{
		cells:    make(map[uint64][]geoEntry),
		points:   make(map[uint32]Point),
		cellSize: cellSizeDegrees,
	}
}

// Add adds a point to the index
func (idx *Index) Add(docID uint32, p Point) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	cellID := idx.cellID(p)
	idx.cells[cellID] = append(idx.cells[cellID], geoEntry{DocID: docID, Point: p})
	idx.points[docID] = p
}

// Remove removes a document from the index
func (idx *Index) Remove(docID uint32) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	p, exists := idx.points[docID]
	if !exists {
		return
	}

	cellID := idx.cellID(p)
	entries := idx.cells[cellID]
	newEntries := entries[:0]
	for _, e := range entries {
		if e.DocID != docID {
			newEntries = append(newEntries, e)
		}
	}
	if len(newEntries) == 0 {
		delete(idx.cells, cellID)
	} else {
		idx.cells[cellID] = newEntries
	}

	delete(idx.points, docID)
}

// SearchRadius finds all points within radius of center
func (idx *Index) SearchRadius(center Point, radiusMeters float64, limit int) []GeoResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Calculate cell range to search
	radiusDegrees := radiusMeters / 111000 // ~111km per degree latitude
	cellRange := int(math.Ceil(radiusDegrees / idx.cellSize))

	centerCellLat := int(center.Lat / idx.cellSize)
	centerCellLon := int(center.Lon / idx.cellSize)

	var results []GeoResult

	// Search neighboring cells
	for dLat := -cellRange; dLat <= cellRange; dLat++ {
		for dLon := -cellRange; dLon <= cellRange; dLon++ {
			cellLat := centerCellLat + dLat
			cellLon := centerCellLon + dLon
			// #nosec G115 - values are bounded by valid lat/lon ranges
			cellID := uint64(cellLat+900)*3600 + uint64(cellLon+1800)

			for _, entry := range idx.cells[cellID] {
				dist := DistanceMeters(center, entry.Point)
				if dist <= radiusMeters {
					results = append(results, GeoResult{
						DocID:    entry.DocID,
						Point:    entry.Point,
						Distance: dist,
					})
				}
			}
		}
	}

	// Sort by distance
	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// SearchBoundingBox finds all points within a bounding box
func (idx *Index) SearchBoundingBox(bb BoundingBox, limit int) []GeoResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []GeoResult

	// Calculate cell range
	minCellLat := int(bb.BottomRight.Lat / idx.cellSize)
	maxCellLat := int(bb.TopLeft.Lat / idx.cellSize)
	minCellLon := int(bb.TopLeft.Lon / idx.cellSize)
	maxCellLon := int(bb.BottomRight.Lon / idx.cellSize)

	for cellLat := minCellLat; cellLat <= maxCellLat; cellLat++ {
		for cellLon := minCellLon; cellLon <= maxCellLon; cellLon++ {
			// #nosec G115 - values are bounded by valid lat/lon ranges
			cellID := uint64(cellLat+900)*3600 + uint64(cellLon+1800)

			for _, entry := range idx.cells[cellID] {
				if bb.Contains(entry.Point) {
					results = append(results, GeoResult{
						DocID: entry.DocID,
						Point: entry.Point,
					})
				}
			}
		}
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// SearchPolygon finds all points within a polygon
func (idx *Index) SearchPolygon(poly Polygon, limit int) []GeoResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Get bounding box of polygon
	bb := polygonBoundingBox(poly.Exterior)

	// Search within bounding box first, then filter by polygon
	candidates := idx.SearchBoundingBox(bb, 0)

	var results []GeoResult
	for _, c := range candidates {
		if poly.Contains(c.Point) {
			results = append(results, c)
		}
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// SearchFilter performs a search using a geo filter
func (idx *Index) SearchFilter(filter *Filter, limit int) []GeoResult {
	switch filter.Type {
	case FilterRadius:
		return idx.SearchRadius(filter.Center, filter.Radius, limit)
	case FilterBoundingBox:
		if filter.BoundingBox == nil {
			return nil
		}
		return idx.SearchBoundingBox(*filter.BoundingBox, limit)
	case FilterPolygon:
		if filter.Polygon == nil {
			return nil
		}
		return idx.SearchPolygon(*filter.Polygon, limit)
	default:
		return nil
	}
}

// Get retrieves the point for a document
func (idx *Index) Get(docID uint32) (Point, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	p, exists := idx.points[docID]
	return p, exists
}

// Size returns the number of indexed points
func (idx *Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.points)
}

// cellID computes the grid cell ID for a point
func (idx *Index) cellID(p Point) uint64 {
	cellLat := int(p.Lat / idx.cellSize)
	cellLon := int(p.Lon / idx.cellSize)
	// Offset to handle negative values
	// #nosec G115 - values are bounded by valid lat (-90,90) and lon (-180,180) ranges
	return uint64(cellLat+900)*3600 + uint64(cellLon+1800)
}

// GeoResult represents a search result with distance
type GeoResult struct {
	DocID    uint32  `json:"doc_id"`
	Point    Point   `json:"point"`
	Distance float64 `json:"distance,omitempty"` // Distance in meters
}

// polygonBoundingBox calculates the bounding box of a polygon
func polygonBoundingBox(points []Point) BoundingBox {
	if len(points) == 0 {
		return BoundingBox{}
	}

	minLat, maxLat := points[0].Lat, points[0].Lat
	minLon, maxLon := points[0].Lon, points[0].Lon

	for _, p := range points[1:] {
		if p.Lat < minLat {
			minLat = p.Lat
		}
		if p.Lat > maxLat {
			maxLat = p.Lat
		}
		if p.Lon < minLon {
			minLon = p.Lon
		}
		if p.Lon > maxLon {
			maxLon = p.Lon
		}
	}

	return BoundingBox{
		TopLeft:     Point{Lat: maxLat, Lon: minLon},
		BottomRight: Point{Lat: minLat, Lon: maxLon},
	}
}

// =============================================================================
// Geohash utilities (for compatibility)
// =============================================================================

const base32 = "0123456789bcdefghjkmnpqrstuvwxyz"

// Encode encodes a point to a geohash string
func Encode(p Point, precision int) string {
	if precision < 1 {
		precision = 12
	}

	minLat, maxLat := -90.0, 90.0
	minLon, maxLon := -180.0, 180.0

	var hash []byte
	var bits uint
	var bitCount int

	for len(hash) < precision {
		if bitCount%2 == 0 {
			// Even bit: longitude
			mid := (minLon + maxLon) / 2
			if p.Lon >= mid {
				bits = bits*2 + 1
				minLon = mid
			} else {
				bits = bits * 2
				maxLon = mid
			}
		} else {
			// Odd bit: latitude
			mid := (minLat + maxLat) / 2
			if p.Lat >= mid {
				bits = bits*2 + 1
				minLat = mid
			} else {
				bits = bits * 2
				maxLat = mid
			}
		}

		bitCount++
		if bitCount == 5 {
			hash = append(hash, base32[bits])
			bits = 0
			bitCount = 0
		}
	}

	return string(hash)
}

// =============================================================================
// JSON helpers
// =============================================================================

// ParsePoint parses a point from JSON or map
func ParsePoint(v interface{}) (Point, error) {
	switch val := v.(type) {
	case Point:
		return val, nil
	case *Point:
		return *val, nil
	case map[string]interface{}:
		p := Point{}
		if lat, ok := val["lat"].(float64); ok {
			p.Lat = lat
		}
		if lon, ok := val["lon"].(float64); ok {
			p.Lon = lon
		}
		return p, nil
	case string:
		var p Point
		if err := json.Unmarshal([]byte(val), &p); err != nil {
			return Point{}, err
		}
		return p, nil
	default:
		return Point{}, nil
	}
}
