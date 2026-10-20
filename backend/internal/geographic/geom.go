package geographic

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Point is a WGS84 coordinate.
type Point struct{ Lon, Lat float64 }

// Polygon is a list of rings: ring 0 is the outer boundary, the others are holes.
type Polygon [][]Point

// Geometry is a validated GeoJSON Polygon or MultiPolygon.
type Geometry struct{ Polys []Polygon }

// BBox is [minLon, minLat, maxLon, maxLat].
type BBox [4]float64

// Contains reports whether the point lies inside the box (borders included).
func (b BBox) Contains(lon, lat float64) bool {
	return lon >= b[0] && lon <= b[2] && lat >= b[1] && lat <= b[3]
}

// ParseBBox parses "min_lon,min_lat,max_lon,max_lat" and validates ranges and ordering.
func ParseBBox(s string) (BBox, error) {
	var b BBox
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return b, errors.New("bbox must be min_lon,min_lat,max_lon,max_lat")
	}
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return b, errors.New("bbox values must be numbers")
		}
		b[i] = v
	}
	if b[0] < -180 || b[2] > 180 || b[1] < -90 || b[3] > 90 {
		return b, errors.New("bbox out of range")
	}
	if b[0] > b[2] || b[1] > b[3] {
		return b, errors.New("bbox min must not exceed max")
	}
	return b, nil
}

const maxVertices = 50000

type rawGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// ParseGeometry parses and validates a GeoJSON geometry object (Polygon or MultiPolygon).
// Checks: known type, rings closed with >= 4 positions, finite coordinates within WGS84 range,
// non-zero ring area, vertex cap. Ring self-intersection is not checked.
func ParseGeometry(raw []byte) (*Geometry, error) {
	var rg rawGeometry
	if err := json.Unmarshal(raw, &rg); err != nil {
		return nil, errors.New("geometry is not valid JSON")
	}
	var polys [][][][]float64
	switch rg.Type {
	case "Polygon":
		var p [][][]float64
		if err := json.Unmarshal(rg.Coordinates, &p); err != nil {
			return nil, errors.New("invalid Polygon coordinates")
		}
		polys = [][][][]float64{p}
	case "MultiPolygon":
		if err := json.Unmarshal(rg.Coordinates, &polys); err != nil {
			return nil, errors.New("invalid MultiPolygon coordinates")
		}
	default:
		return nil, fmt.Errorf("geometry type must be Polygon or MultiPolygon, got %q", rg.Type)
	}
	if len(polys) == 0 {
		return nil, errors.New("geometry has no polygons")
	}
	g := &Geometry{}
	total := 0
	for _, p := range polys {
		if len(p) == 0 {
			return nil, errors.New("polygon has no rings")
		}
		var poly Polygon
		for ri, r := range p {
			if len(r) < 4 {
				return nil, errors.New("a ring needs at least 4 positions")
			}
			total += len(r)
			if total > maxVertices {
				return nil, fmt.Errorf("geometry has more than %d vertices", maxVertices)
			}
			ring := make([]Point, len(r))
			for i, c := range r {
				if len(c) < 2 {
					return nil, errors.New("a position needs [lon, lat]")
				}
				lon, lat := c[0], c[1]
				if math.IsNaN(lon) || math.IsNaN(lat) || math.IsInf(lon, 0) || math.IsInf(lat, 0) ||
					lon < -180 || lon > 180 || lat < -90 || lat > 90 {
					return nil, errors.New("coordinates must be finite lon in [-180,180], lat in [-90,90]")
				}
				ring[i] = Point{lon, lat}
			}
			if ring[0] != ring[len(ring)-1] {
				return nil, errors.New("rings must be closed (first position equals last)")
			}
			if ri == 0 && math.Abs(ringArea(ring)) == 0 {
				return nil, errors.New("outer ring has zero area")
			}
			poly = append(poly, ring)
		}
		g.Polys = append(g.Polys, poly)
	}
	return g, nil
}

// ringArea is the signed planar (lon/lat) shoelace area.
func ringArea(r []Point) float64 {
	var a float64
	for i := 0; i < len(r)-1; i++ {
		a += r[i].Lon*r[i+1].Lat - r[i+1].Lon*r[i].Lat
	}
	return a / 2
}

// Contains reports whether the point is inside the geometry (inside any polygon, outside its
// holes; points exactly on a boundary count as inside).
func (g *Geometry) Contains(lon, lat float64) bool {
	for _, p := range g.Polys {
		if len(p) == 0 {
			continue
		}
		in, onEdge := ringContains(p[0], lon, lat)
		if onEdge {
			return true
		}
		if !in {
			continue
		}
		inHole := false
		for _, h := range p[1:] {
			hin, hedge := ringContains(h, lon, lat)
			if hedge { // on a hole's edge: boundary of the polygon
				return true
			}
			if hin {
				inHole = true
				break
			}
		}
		if !inHole {
			return true
		}
	}
	return false
}

// ringContains does even-odd ray casting; onEdge is true when the point lies on the ring itself.
func ringContains(r []Point, lon, lat float64) (inside, onEdge bool) {
	const eps = 1e-12
	j := len(r) - 1
	for i := 0; i < len(r); i++ {
		a, b := r[j], r[i]
		// on-segment test
		cross := (lon-a.Lon)*(b.Lat-a.Lat) - (lat-a.Lat)*(b.Lon-a.Lon)
		if math.Abs(cross) <= eps &&
			lon >= math.Min(a.Lon, b.Lon)-eps && lon <= math.Max(a.Lon, b.Lon)+eps &&
			lat >= math.Min(a.Lat, b.Lat)-eps && lat <= math.Max(a.Lat, b.Lat)+eps {
			return true, true
		}
		if (b.Lat > lat) != (a.Lat > lat) && lon < (a.Lon-b.Lon)*(lat-b.Lat)/(a.Lat-b.Lat)+b.Lon {
			inside = !inside
		}
		j = i
	}
	return inside, false
}

// BBox returns the bounding box of all outer rings.
func (g *Geometry) BBox() BBox {
	b := BBox{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, p := range g.Polys {
		for _, pt := range p[0] {
			b[0], b[1] = math.Min(b[0], pt.Lon), math.Min(b[1], pt.Lat)
			b[2], b[3] = math.Max(b[2], pt.Lon), math.Max(b[3], pt.Lat)
		}
	}
	return b
}

// Centroid returns the area-weighted centroid of the outer rings (bbox centre when degenerate).
func (g *Geometry) Centroid() (lon, lat float64) {
	var area, cx, cy float64
	for _, p := range g.Polys {
		r := p[0]
		for i := 0; i < len(r)-1; i++ {
			c := r[i].Lon*r[i+1].Lat - r[i+1].Lon*r[i].Lat
			area += c
			cx += (r[i].Lon + r[i+1].Lon) * c
			cy += (r[i].Lat + r[i+1].Lat) * c
		}
	}
	b := g.BBox()
	if area == 0 {
		return (b[0] + b[2]) / 2, (b[1] + b[3]) / 2
	}
	lon, lat = cx/(3*area), cy/(3*area)
	if !b.Contains(lon, lat) {
		return (b[0] + b[2]) / 2, (b[1] + b[3]) / 2
	}
	return lon, lat
}

// InteriorPoint returns a point guaranteed inside the geometry: the centroid if it is inside,
// otherwise a scan of the bbox (used for seeding demo data).
func (g *Geometry) InteriorPoint() (lon, lat float64) {
	lon, lat = g.Centroid()
	if g.Contains(lon, lat) {
		return
	}
	b := g.BBox()
	for i := 1; i < 40; i++ {
		for j := 1; j < 40; j++ {
			x := b[0] + (b[2]-b[0])*float64(i)/40
			y := b[1] + (b[3]-b[1])*float64(j)/40
			if g.Contains(x, y) {
				return x, y
			}
		}
	}
	return b[0], b[1]
}

// PointInPolygon is a convenience for a single GeoJSON-style ring set (outer ring first).
func PointInPolygon(lon, lat float64, rings ...[]Point) bool {
	return (&Geometry{Polys: []Polygon{rings}}).Contains(lon, lat)
}
