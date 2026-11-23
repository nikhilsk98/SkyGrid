package grid

import (
	"errors"
	"fmt"
	"math"
)

// Step is one (cell, time slot) a drone must reserve.
type Step struct {
	Cell CellID `json:"cell"`
	Slot int64  `json:"slot"`
}

// ErrInvalidSlot is wrapped when a path's slots would be negative or overflow.
var ErrInvalidSlot = errors.New("invalid slot")

// PathLength returns |dx| + |dy| + 1, the number of steps Path returns.
func PathLength(from, to Point) int {
	return abs(to.X-from.X) + abs(to.Y-from.Y) + 1
}

// Path returns the deterministic L-shaped path from docs/design.md section 2:
// move along x first, then along y, one cell per step. Both endpoints are
// included and step i is reserved at slot startSlot+i, so the path has
// |dx|+|dy|+1 steps and ends at slot startSlot+len-1. When from == to the
// path is the single cell at startSlot.
func (g *Grid) Path(from, to Point, startSlot int64) ([]Step, error) {
	if !g.Contains(from) {
		return nil, fmt.Errorf("path from: %w", g.pointErr(from))
	}
	if !g.Contains(to) {
		return nil, fmt.Errorf("path to: %w", g.pointErr(to))
	}
	n := PathLength(from, to)
	if startSlot < 0 {
		return nil, fmt.Errorf("grid: start slot %d: %w (must be >= 0)", startSlot, ErrInvalidSlot)
	}
	if startSlot > math.MaxInt64-int64(n-1) {
		return nil, fmt.Errorf("grid: start slot %d: %w (end slot overflows)", startSlot, ErrInvalidSlot)
	}

	steps := make([]Step, 0, n)
	p := from
	add := func() {
		steps = append(steps, Step{Cell: g.cellID(p), Slot: startSlot + int64(len(steps))})
	}
	add()
	dx, dy := sign(to.X-from.X), sign(to.Y-from.Y)
	for p.X != to.X {
		p.X += dx
		add()
	}
	for p.Y != to.Y {
		p.Y += dy
		add()
	}
	return steps, nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
