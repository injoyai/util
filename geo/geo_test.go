package geo

import "testing"

func TestSegmentBoundaryCases(t *testing.T) {
	for _, tc := range []struct {
		a, b Line
		want bool
	}{
		{Line{{0, 0}, {2, 0}}, Line{{1, 0}, {3, 0}}, true},
		{Line{{0, 0}, {1, 0}}, Line{{1, 0}, {1, 1}}, true},
		{Line{{0, 0}, {1, 0}}, Line{{2, 0}, {3, 0}}, false},
		{Line{{0, 0}, {2, 2}}, Line{{0, 2}, {2, 0}}, true},
		{Line{{1, 1}, {1, 1}}, Line{{0, 0}, {2, 2}}, true},
		{Line{{3, 3}, {3, 3}}, Line{{0, 0}, {2, 2}}, false},
	} {
		if got := tc.a.Intersect(tc.b); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	if PointInLine(Point{1, 0.001}, Line{{0, 0}, {2, 0}}) {
		t.Fatal("off-line point")
	}
}

func TestPolygonContainmentAndContact(t *testing.T) {
	a := Polygon{{0, 0}, {4, 0}, {4, 4}, {0, 4}}
	if !a.Contains(Point{0, 2}) || !a.Contains(Point{2, 2}) || a.Contains(Point{5, 2}) {
		t.Fatal("contains")
	}
	if !a.Intersect(Polygon{{1, 1}, {2, 1}, {2, 2}, {1, 2}}) {
		t.Fatal("contained polygon not detected")
	}
	if !a.Intersect(Polygon{{4, 1}, {5, 1}, {5, 2}, {4, 2}}) {
		t.Fatal("touch not detected")
	}
	if a.Intersect(Polygon{{5, 1}, {6, 1}, {6, 2}, {5, 2}}) || a.Intersect(nil) {
		t.Fatal("false intersection")
	}
}
