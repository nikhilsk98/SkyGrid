package grid

import (
	"errors"
	"testing"

	"github.com/nikhilsk98/SkyGrid/internal/config"
)

// defaultGrid matches config/skygrid.yaml: 100x100 cells, 5x4 regions of 20x25.
func defaultGrid(t *testing.T) *Grid {
	t.Helper()
	g, err := FromConfig(config.Grid{Width: 100, Height: 100, RegionCols: 5, RegionRows: 4, SlotDurationMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestNewRejectsBadDimensions(t *testing.T) {
	tests := []struct {
		name             string
		w, h, cols, rows int
	}{
		{"zero width", 0, 100, 5, 4},
		{"negative height", 100, -1, 5, 4},
		{"zero cols", 100, 100, 0, 4},
		{"zero rows", 100, 100, 5, 0},
		{"width not divisible", 101, 100, 5, 4},
		{"height not divisible", 100, 101, 5, 4},
		{"too many cells", 1 << 16, 1 << 16, 1, 1},
	}
	for _, tt := range tests {
		if _, err := New(tt.w, tt.h, tt.cols, tt.rows); err == nil {
			t.Errorf("%s: New succeeded, want error", tt.name)
		}
	}
}

func TestDimensions(t *testing.T) {
	g := defaultGrid(t)
	if g.Width() != 100 || g.Height() != 100 || g.CellCount() != 10000 || g.RegionCount() != 20 {
		t.Errorf("got %dx%d, %d cells, %d regions", g.Width(), g.Height(), g.CellCount(), g.RegionCount())
	}
	if w, h := g.RegionSize(); w != 20 || h != 25 {
		t.Errorf("RegionSize() = %d,%d, want 20,25", w, h)
	}
}

func TestCellAndRegion(t *testing.T) {
	g := defaultGrid(t)
	tests := []struct {
		name   string
		p      Point
		cell   CellID
		region RegionID
	}{
		// Grid corners.
		{"top-left corner", Point{0, 0}, 0, 0},
		{"top-right corner", Point{99, 0}, 99, 4},
		{"bottom-left corner", Point{0, 99}, 9900, 15},
		{"bottom-right corner", Point{99, 99}, 9999, 19},
		// Region boundaries along x (region_w = 20).
		{"x: last of region 0", Point{19, 0}, 19, 0},
		{"x: first of region 1", Point{20, 0}, 20, 1},
		{"x: last of region 1", Point{39, 0}, 39, 1},
		{"x: first of region 2", Point{40, 0}, 40, 2},
		{"x: last of region 3", Point{79, 10}, 1079, 3},
		{"x: first of region 4", Point{80, 10}, 1080, 4},
		// Region boundaries along y (region_h = 25).
		{"y: last of region 0", Point{0, 24}, 2400, 0},
		{"y: first of region 5", Point{0, 25}, 2500, 5},
		{"y: last of region 5", Point{0, 49}, 4900, 5},
		{"y: first of region 10", Point{0, 50}, 5000, 10},
		{"y: last of region 14", Point{99, 74}, 7499, 14},
		{"y: first of region 19", Point{99, 75}, 7599, 19},
		// Both axes at once.
		{"last cell of region 0", Point{19, 24}, 2419, 0},
		{"diagonal neighbor region 6", Point{20, 25}, 2520, 6},
		{"interior", Point{57, 63}, 6357, 12},
		// api.md example endpoints.
		{"api from", Point{3, 10}, 1003, 0},
		{"api to", Point{80, 62}, 6280, 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !g.Contains(tt.p) {
				t.Fatalf("Contains(%v) = false", tt.p)
			}
			cell, err := g.CellID(tt.p)
			if err != nil || cell != tt.cell {
				t.Errorf("CellID(%v) = %d, %v; want %d", tt.p, cell, err, tt.cell)
			}
			p, err := g.PointOf(tt.cell)
			if err != nil || p != tt.p {
				t.Errorf("PointOf(%d) = %v, %v; want %v", tt.cell, p, err, tt.p)
			}
			r, err := g.RegionOf(tt.p)
			if err != nil || r != tt.region {
				t.Errorf("RegionOf(%v) = %d, %v; want %d", tt.p, r, err, tt.region)
			}
			r, err = g.RegionOfCell(tt.cell)
			if err != nil || r != tt.region {
				t.Errorf("RegionOfCell(%d) = %d, %v; want %d", tt.cell, r, err, tt.region)
			}
			b, err := g.RegionBounds(tt.region)
			if err != nil || !b.Contains(tt.p) {
				t.Errorf("RegionBounds(%d) = %+v, %v; does not contain %v", tt.region, b, err, tt.p)
			}
		})
	}
}

func TestRegionFormulaNonSquare(t *testing.T) {
	// 12x6 grid, 3x2 regions of 4x3: checks region_w and region_h are not mixed up.
	g, err := New(12, 6, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		p    Point
		cell CellID
		want RegionID
	}{
		{Point{3, 2}, 27, 0},
		{Point{4, 2}, 28, 1},
		{Point{3, 3}, 39, 3},
		{Point{11, 5}, 71, 5},
		{Point{8, 0}, 8, 2},
	}
	for _, tt := range tests {
		if c, _ := g.CellID(tt.p); c != tt.cell {
			t.Errorf("CellID(%v) = %d, want %d", tt.p, c, tt.cell)
		}
		if r, _ := g.RegionOf(tt.p); r != tt.want {
			t.Errorf("RegionOf(%v) = %d, want %d", tt.p, r, tt.want)
		}
	}
}

func TestEveryCellRoundTrips(t *testing.T) {
	g := defaultGrid(t)
	perRegion := make(map[RegionID]int)
	for id := CellID(0); int(id) < g.CellCount(); id++ {
		p, err := g.PointOf(id)
		if err != nil {
			t.Fatal(err)
		}
		back, err := g.CellID(p)
		if err != nil || back != id {
			t.Fatalf("CellID(PointOf(%d)) = %d, %v", id, back, err)
		}
		r, _ := g.RegionOf(p)
		perRegion[r]++
	}
	if len(perRegion) != 20 {
		t.Fatalf("cells fall in %d regions, want 20", len(perRegion))
	}
	for r, n := range perRegion {
		if n != 20*25 {
			t.Errorf("region %d has %d cells, want 500", r, n)
		}
	}
}

func TestRegionBounds(t *testing.T) {
	g := defaultGrid(t)
	tests := []struct {
		r    RegionID
		want Bounds
	}{
		{0, Bounds{Point{0, 0}, Point{20, 25}}},
		{4, Bounds{Point{80, 0}, Point{100, 25}}},
		{5, Bounds{Point{0, 25}, Point{20, 50}}},
		{7, Bounds{Point{40, 25}, Point{60, 50}}},
		{19, Bounds{Point{80, 75}, Point{100, 100}}},
	}
	for _, tt := range tests {
		b, err := g.RegionBounds(tt.r)
		if err != nil || b != tt.want {
			t.Errorf("RegionBounds(%d) = %+v, %v; want %+v", tt.r, b, err, tt.want)
		}
		// Every cell inside the bounds maps back to the region, and the cells
		// just outside do not.
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if r, _ := g.RegionOf(Point{x, y}); r != tt.r {
					t.Fatalf("RegionOf(%d,%d) = %d, want %d", x, y, r, tt.r)
				}
			}
		}
		for _, out := range []Point{{b.Max.X, b.Min.Y}, {b.Min.X, b.Max.Y}, {b.Min.X - 1, b.Min.Y}, {b.Min.X, b.Min.Y - 1}} {
			if r, err := g.RegionOf(out); err == nil && r == tt.r {
				t.Errorf("point %v outside bounds maps to region %d", out, tt.r)
			}
		}
	}
	for _, bad := range []RegionID{-1, 20, 100} {
		if _, err := g.RegionBounds(bad); !errors.Is(err, ErrOutOfRange) {
			t.Errorf("RegionBounds(%d) err = %v, want ErrOutOfRange", bad, err)
		}
	}
}

func TestInvalidPointsRejected(t *testing.T) {
	g := defaultGrid(t)
	bad := []Point{{-1, 0}, {0, -1}, {100, 0}, {0, 100}, {100, 100}, {-1, -1}, {1 << 40, 0}}
	for _, p := range bad {
		if g.Contains(p) {
			t.Errorf("Contains(%v) = true", p)
		}
		if _, err := g.CellID(p); !errors.Is(err, ErrOutOfRange) {
			t.Errorf("CellID(%v) err = %v, want ErrOutOfRange", p, err)
		}
		if _, err := g.RegionOf(p); !errors.Is(err, ErrOutOfRange) {
			t.Errorf("RegionOf(%v) err = %v, want ErrOutOfRange", p, err)
		}
	}
	for _, id := range []CellID{-1, 10000, 1 << 30} {
		if g.ContainsCell(id) {
			t.Errorf("ContainsCell(%d) = true", id)
		}
		if _, err := g.PointOf(id); !errors.Is(err, ErrOutOfRange) {
			t.Errorf("PointOf(%d) err = %v, want ErrOutOfRange", id, err)
		}
		if _, err := g.RegionOfCell(id); !errors.Is(err, ErrOutOfRange) {
			t.Errorf("RegionOfCell(%d) err = %v, want ErrOutOfRange", id, err)
		}
	}
}
