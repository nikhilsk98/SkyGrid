package grid

import (
	"errors"
	"math"
	"testing"
)

func cells(steps []Step) []CellID {
	out := make([]CellID, len(steps))
	for i, s := range steps {
		out[i] = s.Cell
	}
	return out
}

func equalCells(a, b []CellID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestPathExact pins the exact cell sequence for every direction of travel on
// the default 100-wide grid (cell = y*100 + x).
func TestPathExact(t *testing.T) {
	g := defaultGrid(t)
	tests := []struct {
		name     string
		from, to Point
		want     []CellID
	}{
		{"same cell", Point{5, 5}, Point{5, 5}, []CellID{505}},
		{"east", Point{1, 2}, Point{4, 2}, []CellID{201, 202, 203, 204}},
		{"west", Point{4, 2}, Point{1, 2}, []CellID{204, 203, 202, 201}},
		{"south (+y)", Point{3, 1}, Point{3, 4}, []CellID{103, 203, 303, 403}},
		{"north (-y)", Point{3, 4}, Point{3, 1}, []CellID{403, 303, 203, 103}},
		{"south-east", Point{1, 1}, Point{3, 3}, []CellID{101, 102, 103, 203, 303}},
		{"north-east", Point{1, 3}, Point{3, 1}, []CellID{301, 302, 303, 203, 103}},
		{"south-west", Point{3, 1}, Point{1, 3}, []CellID{103, 102, 101, 201, 301}},
		{"north-west", Point{3, 3}, Point{1, 1}, []CellID{303, 302, 301, 201, 101}},
		{"one step east", Point{0, 0}, Point{1, 0}, []CellID{0, 1}},
		{"one step south", Point{0, 0}, Point{0, 1}, []CellID{0, 100}},
		{"across x region boundary", Point{18, 0}, Point{21, 0}, []CellID{18, 19, 20, 21}},
		{"across y region boundary", Point{0, 23}, Point{0, 26}, []CellID{2300, 2400, 2500, 2600}},
		{"corner to corner on top edge", Point{99, 0}, Point{98, 1}, []CellID{99, 98, 198}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := g.Path(tt.from, tt.to, 1000)
			if err != nil {
				t.Fatal(err)
			}
			if got := cells(steps); !equalCells(got, tt.want) {
				t.Errorf("cells = %v, want %v", got, tt.want)
			}
			for i, s := range steps {
				if s.Slot != 1000+int64(i) {
					t.Errorf("step %d slot = %d, want %d", i, s.Slot, 1000+i)
				}
			}
		})
	}
}

// TestPathAPIExample checks every number in the worked examples in
// docs/api.md for the request (3,10) -> (80,62).
func TestPathAPIExample(t *testing.T) {
	g := defaultGrid(t)
	steps, err := g.Path(Point{3, 10}, Point{80, 62}, 1795000102)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 130 || PathLength(Point{3, 10}, Point{80, 62}) != 130 {
		t.Fatalf("path length = %d, want 130", len(steps))
	}
	if end := steps[len(steps)-1].Slot; end != 1795000231 {
		t.Errorf("end slot = %d, want 1795000231", end)
	}

	// 200 example: regions crossed, and shards under the seeded
	// region_id % 4 placement (migrations/metadata/002_seed_routing.sql).
	regions := map[RegionID]bool{}
	shards := map[int]bool{}
	for _, s := range steps {
		r, err := g.RegionOfCell(s.Cell)
		if err != nil {
			t.Fatal(err)
		}
		regions[r] = true
		shards[int(r)%4] = true
	}
	for _, r := range []RegionID{0, 1, 2, 3, 4, 9, 14} {
		if !regions[r] {
			t.Errorf("path does not cross region %d", r)
		}
	}
	if len(regions) != 7 || len(shards) != 4 {
		t.Errorf("regions = %v, shards = %v; want 7 regions on shards 0-3", regions, shards)
	}

	// 409 example: fifth attempt starts at 1795000104 and fails at cell 4580.
	late, err := g.Path(Point{3, 10}, Point{80, 62}, 1795000104)
	if err != nil {
		t.Fatal(err)
	}
	if s := late[112]; s.Cell != 4580 || s.Slot != 1795000216 {
		t.Errorf("step 112 = %+v, want cell 4580 slot 1795000216", s)
	}
	// The corner of the L is (80,10), reached after 77 x-moves.
	if steps[77].Cell != 1080 {
		t.Errorf("corner cell = %d, want 1080", steps[77].Cell)
	}
}

// TestPathCornersAndDirections checks the path invariants between all four
// grid corners and in every direction of travel.
func TestPathCornersAndDirections(t *testing.T) {
	g := defaultGrid(t)
	corners := []Point{{0, 0}, {99, 0}, {0, 99}, {99, 99}}
	mid := Point{50, 50}
	var pairs [][2]Point
	for _, a := range corners {
		for _, b := range corners {
			pairs = append(pairs, [2]Point{a, b})
		}
		pairs = append(pairs, [2]Point{a, mid}, [2]Point{mid, a})
	}
	for _, d := range []Point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}, {0, 0}} {
		pairs = append(pairs, [2]Point{mid, {mid.X + 17*d.X, mid.Y + 31*d.Y}})
	}
	for _, pr := range pairs {
		checkPath(t, g, pr[0], pr[1], 42)
	}
}

// TestPathExhaustiveSmallGrid checks every (from, to) pair on a small grid.
func TestPathExhaustiveSmallGrid(t *testing.T) {
	g, err := New(6, 4, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	for fy := range 4 {
		for fx := range 6 {
			for ty := range 4 {
				for tx := range 6 {
					checkPath(t, g, Point{fx, fy}, Point{tx, ty}, 7)
				}
			}
		}
	}
}

// checkPath verifies every property design.md section 2 promises.
func checkPath(t *testing.T, g *Grid, from, to Point, start int64) {
	t.Helper()
	steps, err := g.Path(from, to, start)
	if err != nil {
		t.Fatalf("Path(%v, %v): %v", from, to, err)
	}
	wantLen := abs(to.X-from.X) + abs(to.Y-from.Y) + 1
	if len(steps) != wantLen || PathLength(from, to) != wantLen {
		t.Fatalf("Path(%v, %v): length %d, want %d", from, to, len(steps), wantLen)
	}
	if c, _ := g.CellID(from); steps[0].Cell != c {
		t.Errorf("Path(%v, %v): first cell %d, want %d", from, to, steps[0].Cell, c)
	}
	if c, _ := g.CellID(to); steps[len(steps)-1].Cell != c {
		t.Errorf("Path(%v, %v): last cell %d, want %d", from, to, steps[len(steps)-1].Cell, c)
	}
	if end := steps[len(steps)-1].Slot; end != start+int64(wantLen)-1 {
		t.Errorf("Path(%v, %v): end slot %d, want %d", from, to, end, start+int64(wantLen)-1)
	}
	seen := make(map[CellID]bool, len(steps))
	turned := false // true once the path has started moving along y
	prev, _ := g.PointOf(steps[0].Cell)
	for i, s := range steps {
		if s.Slot != start+int64(i) {
			t.Fatalf("Path(%v, %v): step %d slot %d, want %d", from, to, i, s.Slot, start+int64(i))
		}
		if seen[s.Cell] {
			t.Fatalf("Path(%v, %v): cell %d visited twice", from, to, s.Cell)
		}
		seen[s.Cell] = true
		if i == 0 {
			continue
		}
		p, err := g.PointOf(s.Cell)
		if err != nil {
			t.Fatal(err)
		}
		dx, dy := p.X-prev.X, p.Y-prev.Y
		if abs(dx)+abs(dy) != 1 {
			t.Fatalf("Path(%v, %v): step %d jumps from %v to %v", from, to, i, prev, p)
		}
		if dy != 0 {
			turned = true
			if p.X != to.X {
				t.Fatalf("Path(%v, %v): moved along y at x=%d before reaching x=%d", from, to, p.X, to.X)
			}
		} else if turned {
			t.Fatalf("Path(%v, %v): moved along x after moving along y", from, to)
		}
		prev = p
	}
}

func TestPathIsDeterministic(t *testing.T) {
	g := defaultGrid(t)
	a, _ := g.Path(Point{90, 3}, Point{2, 97}, 5)
	b, _ := g.Path(Point{90, 3}, Point{2, 97}, 5)
	if len(a) != len(b) {
		t.Fatal("lengths differ")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("step %d differs: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestPathRejectsInvalidInput(t *testing.T) {
	g := defaultGrid(t)
	tests := []struct {
		name     string
		from, to Point
		start    int64
		want     error
	}{
		{"from x negative", Point{-1, 0}, Point{5, 5}, 0, ErrOutOfRange},
		{"from y too big", Point{0, 100}, Point{5, 5}, 0, ErrOutOfRange},
		{"to x too big", Point{0, 0}, Point{100, 5}, 0, ErrOutOfRange},
		{"to y negative", Point{0, 0}, Point{5, -1}, 0, ErrOutOfRange},
		{"negative start slot", Point{0, 0}, Point{5, 5}, -1, ErrInvalidSlot},
		{"end slot overflows", Point{0, 0}, Point{5, 0}, math.MaxInt64 - 4, ErrInvalidSlot},
	}
	for _, tt := range tests {
		steps, err := g.Path(tt.from, tt.to, tt.start)
		if !errors.Is(err, tt.want) || steps != nil {
			t.Errorf("%s: got %v, %v; want error %v", tt.name, steps, err, tt.want)
		}
	}
	// The largest start slot that still fits is accepted.
	steps, err := g.Path(Point{0, 0}, Point{5, 0}, math.MaxInt64-5)
	if err != nil || steps[len(steps)-1].Slot != math.MaxInt64 {
		t.Errorf("max start slot: %v, %v", steps, err)
	}
}
