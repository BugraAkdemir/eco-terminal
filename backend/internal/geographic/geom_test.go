package geographic

import "testing"

const squareWithHole = `{"type":"Polygon","coordinates":[
 [[0,0],[10,0],[10,10],[0,10],[0,0]],
 [[4,4],[6,4],[6,6],[4,6],[4,4]]]}`

func TestPointInPolygon(t *testing.T) {
	g, err := ParseGeometry([]byte(squareWithHole))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		lon, lat float64
		want     bool
	}{
		{1, 1, true},
		{5, 5, false}, // inside the hole
		{4, 5, true},  // on the hole edge counts as boundary
		{0, 5, true},  // on outer edge
		{10, 10, true},
		{-1, 5, false},
		{11, 5, false},
		{5, 11, false},
		{9.999, 0.001, true},
	}
	for _, c := range cases {
		if got := g.Contains(c.lon, c.lat); got != c.want {
			t.Errorf("Contains(%v,%v)=%v want %v", c.lon, c.lat, got, c.want)
		}
	}
}

func TestMultiPolygonAndBBox(t *testing.T) {
	g, err := ParseGeometry([]byte(`{"type":"MultiPolygon","coordinates":[
	 [[[0,0],[1,0],[1,1],[0,1],[0,0]]],
	 [[[5,5],[6,5],[6,6],[5,6],[5,5]]]]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !g.Contains(0.5, 0.5) || !g.Contains(5.5, 5.5) || g.Contains(3, 3) {
		t.Fatal("multipolygon containment wrong")
	}
	if b := g.BBox(); b != (BBox{0, 0, 6, 6}) {
		t.Fatalf("bbox %v", b)
	}
	lon, lat := g.InteriorPoint()
	if !g.Contains(lon, lat) {
		t.Fatalf("interior point %v,%v not inside", lon, lat)
	}
}

func TestConcaveCentroidFallback(t *testing.T) {
	// U shape: centroid falls in the notch
	g, err := ParseGeometry([]byte(`{"type":"Polygon","coordinates":[[[0,0],[9,0],[9,9],[6,9],[6,3],[3,3],[3,9],[0,9],[0,0]]]}`))
	if err != nil {
		t.Fatal(err)
	}
	lon, lat := g.InteriorPoint()
	if !g.Contains(lon, lat) {
		t.Fatalf("interior point %v,%v not inside U shape", lon, lat)
	}
}

func TestParseGeometryRejects(t *testing.T) {
	bad := map[string]string{
		"not json":     `nope`,
		"point":        `{"type":"Point","coordinates":[1,2]}`,
		"open ring":    `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,1]]]}`,
		"open ring 2":  `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,1],[0,2]]]}`,
		"short ring":   `{"type":"Polygon","coordinates":[[[0,0],[1,0],[0,0]]]}`,
		"out of range": `{"type":"Polygon","coordinates":[[[0,0],[200,0],[1,1],[0,0]]]}`,
		"lat range":    `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,91],[0,0]]]}`,
		"zero area":    `{"type":"Polygon","coordinates":[[[0,0],[1,1],[2,2],[0,0]]]}`,
		"empty multi":  `{"type":"MultiPolygon","coordinates":[]}`,
	}
	for name, raw := range bad {
		if _, err := ParseGeometry([]byte(raw)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseBBox(t *testing.T) {
	b, err := ParseBBox("32.5, 41, 34.5,42")
	if err != nil || b != (BBox{32.5, 41, 34.5, 42}) {
		t.Fatalf("%v %v", b, err)
	}
	for _, s := range []string{"", "1,2,3", "a,b,c,d", "5,0,1,1", "0,0,200,1", "0,0,1,95", "NaN,0,1,1"} {
		if _, err := ParseBBox(s); err == nil {
			t.Errorf("bbox %q should fail", s)
		}
	}
}
