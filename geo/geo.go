// Package geo 提供有限坐标的二维点、线段和简单多边形判断。
// 不支持自相交多边形及带洞多边形，不提供多边形布尔裁剪。
package geo

import "math"

// Epsilon 为线段距离及边界判断的绝对容差，单位与坐标一致。
const Epsilon = 1e-9

type Point struct{ X, Y float64 }
type Line [2]Point
type Polygon []Point

func Distance(a, b Point) float64            { return math.Hypot(a.X-b.X, a.Y-b.Y) }
func (p Point) Distance(other Point) float64 { return Distance(p, other) }
func CrossProduct(a, b, c Point) float64     { return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X) }

func signedDistance(p Point, line Line) float64 {
	length := Distance(line[0], line[1])
	if length == 0 {
		return 0
	}
	return CrossProduct(line[0], line[1], p) / length
}

// PointInLine 包括端点和退化成单点的线段。
func PointInLine(p Point, line Line) bool {
	if Distance(line[0], line[1]) == 0 {
		return Distance(p, line[0]) <= Epsilon
	}
	return math.Abs(signedDistance(p, line)) <= Epsilon &&
		p.X >= math.Min(line[0].X, line[1].X)-Epsilon && p.X <= math.Max(line[0].X, line[1].X)+Epsilon &&
		p.Y >= math.Min(line[0].Y, line[1].Y)-Epsilon && p.Y <= math.Max(line[0].Y, line[1].Y)+Epsilon
}

// LineIntersect 包括相交、共线重叠和端点接触。
func LineIntersect(a, b Line) bool {
	if PointInLine(a[0], b) || PointInLine(a[1], b) || PointInLine(b[0], a) || PointInLine(b[1], a) {
		return true
	}
	opposite := func(x, y float64) bool { return x < -Epsilon && y > Epsilon || x > Epsilon && y < -Epsilon }
	return opposite(signedDistance(b[0], a), signedDistance(b[1], a)) && opposite(signedDistance(a[0], b), signedDistance(a[1], b))
}

func (l Line) Intersect(other Line) bool { return LineIntersect(l, other) }

// Contains 使用射线法判断包含，边界也返回 true；少于三个顶点返回 false。
func (p Polygon) Contains(point Point) bool {
	if len(p) < 3 {
		return false
	}
	inside := false
	for i, a := range p {
		b := p[(i+1)%len(p)]
		if PointInLine(point, Line{a, b}) {
			return true
		}
		if (a.Y > point.Y) != (b.Y > point.Y) && point.X < (b.X-a.X)*(point.Y-a.Y)/(b.Y-a.Y)+a.X {
			inside = !inside
		}
	}
	return inside
}

// Intersect 包括边界接触和完全包含关系。
func (p Polygon) Intersect(other Polygon) bool {
	if len(p) < 3 || len(other) < 3 {
		return false
	}
	for i, a := range p {
		for j, b := range other {
			if LineIntersect(Line{a, p[(i+1)%len(p)]}, Line{b, other[(j+1)%len(other)]}) {
				return true
			}
		}
	}
	return p.Contains(other[0]) || other.Contains(p[0])
}
