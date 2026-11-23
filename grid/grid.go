// Package grid implements the grid math and the deterministic path rule from
// docs/design.md sections 1 and 2. Every component that needs a cell ID, a
// region, or a flight path must use this package so they all agree.
package grid

import (
	"errors"
	"fmt"
	"math"

	"github.com/nikhilsk98/SkyGrid/internal/config"
)

// Point is a 0-based grid coordinate: 0 <= X < width, 0 <= Y < height.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func (p Point) String() string { return fmt.Sprintf("(%d,%d)", p.X, p.Y) }

// CellID identifies a cell: cell_id = y*width + x.
type CellID int

// RegionID identifies a logical region:
// region_id = (y/region_h)*region_cols + x/region_w.
type RegionID int

// Bounds is the half-open rectangle of cells [Min.X, Max.X) x [Min.Y, Max.Y),
// like image.Rectangle. The last cell inside is (Max.X-1, Max.Y-1).
type Bounds struct {
	Min, Max Point
}

// Contains reports whether p lies inside b.
func (b Bounds) Contains(p Point) bool {
	return p.X >= b.Min.X && p.X < b.Max.X && p.Y >= b.Min.Y && p.Y < b.Max.Y
}

// ErrOutOfRange is wrapped by every error about a point, cell, or region
// that lies outside the grid.
var ErrOutOfRange = errors.New("out of range")

// Grid holds validated grid dimensions. The zero value is not usable; build
// one with New or FromConfig.
type Grid struct {
	width, height          int
	regionCols, regionRows int
	regionW, regionH       int
}

// New returns a Grid of width x height cells split into regionCols x
// regionRows equal regions.
func New(width, height, regionCols, regionRows int) (*Grid, error) {
	switch {
	case width <= 0 || height <= 0:
		return nil, fmt.Errorf("grid: size must be positive, got %dx%d", width, height)
	case regionCols <= 0 || regionRows <= 0:
		return nil, fmt.Errorf("grid: region layout must be positive, got %dx%d", regionCols, regionRows)
	case width%regionCols != 0:
		return nil, fmt.Errorf("grid: width %d is not divisible by region_cols %d", width, regionCols)
	case height%regionRows != 0:
		return nil, fmt.Errorf("grid: height %d is not divisible by region_rows %d", height, regionRows)
	case int64(width)*int64(height) > math.MaxInt32:
		return nil, fmt.Errorf("grid: %dx%d cells exceed the INTEGER cell_id range", width, height)
	}
	return &Grid{
		width: width, height: height,
		regionCols: regionCols, regionRows: regionRows,
		regionW: width / regionCols, regionH: height / regionRows,
	}, nil
}

// FromConfig builds a Grid from the grid section of skygrid.yaml.
func FromConfig(c config.Grid) (*Grid, error) {
	return New(c.Width, c.Height, c.RegionCols, c.RegionRows)
}

// Width is the number of cells along x.
func (g *Grid) Width() int { return g.width }

// Height is the number of cells along y.
func (g *Grid) Height() int { return g.height }

// CellCount is the total number of cells.
func (g *Grid) CellCount() int { return g.width * g.height }

// RegionCount is the total number of regions (region_cols * region_rows).
func (g *Grid) RegionCount() int { return g.regionCols * g.regionRows }

// RegionSize returns the width and height of every region in cells.
func (g *Grid) RegionSize() (w, h int) { return g.regionW, g.regionH }

// Contains reports whether p is inside the grid.
func (g *Grid) Contains(p Point) bool {
	return p.X >= 0 && p.X < g.width && p.Y >= 0 && p.Y < g.height
}

// ContainsCell reports whether id is a valid cell ID.
func (g *Grid) ContainsCell(id CellID) bool {
	return id >= 0 && int(id) < g.CellCount()
}

// CellID returns y*width + x.
func (g *Grid) CellID(p Point) (CellID, error) {
	if !g.Contains(p) {
		return 0, g.pointErr(p)
	}
	return g.cellID(p), nil
}

// PointOf is the inverse of CellID.
func (g *Grid) PointOf(id CellID) (Point, error) {
	if !g.ContainsCell(id) {
		return Point{}, fmt.Errorf("grid: cell %d %w [0, %d)", id, ErrOutOfRange, g.CellCount())
	}
	return Point{X: int(id) % g.width, Y: int(id) / g.width}, nil
}

// RegionOf returns the region containing p.
func (g *Grid) RegionOf(p Point) (RegionID, error) {
	if !g.Contains(p) {
		return 0, g.pointErr(p)
	}
	return g.regionOf(p), nil
}

// RegionOfCell returns the region containing cell id.
func (g *Grid) RegionOfCell(id CellID) (RegionID, error) {
	p, err := g.PointOf(id)
	if err != nil {
		return 0, err
	}
	return g.regionOf(p), nil
}

// RegionBounds returns the cells covered by region r.
func (g *Grid) RegionBounds(r RegionID) (Bounds, error) {
	if r < 0 || int(r) >= g.RegionCount() {
		return Bounds{}, fmt.Errorf("grid: region %d %w [0, %d)", r, ErrOutOfRange, g.RegionCount())
	}
	col, row := int(r)%g.regionCols, int(r)/g.regionCols
	minP := Point{X: col * g.regionW, Y: row * g.regionH}
	return Bounds{Min: minP, Max: Point{X: minP.X + g.regionW, Y: minP.Y + g.regionH}}, nil
}

func (g *Grid) cellID(p Point) CellID { return CellID(p.Y*g.width + p.X) }

func (g *Grid) regionOf(p Point) RegionID {
	return RegionID((p.Y/g.regionH)*g.regionCols + p.X/g.regionW)
}

func (g *Grid) pointErr(p Point) error {
	return fmt.Errorf("grid: point %v %w for %dx%d grid", p, ErrOutOfRange, g.width, g.height)
}
