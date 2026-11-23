package grid

import (
	"fmt"
	"time"
)

// SlotAt returns the slot containing t: unix_ms / slot_duration_ms (integer
// division), as defined in docs/design.md section 2.
func SlotAt(t time.Time, slotDuration time.Duration) (int64, error) {
	ms := slotDuration.Milliseconds()
	if ms <= 0 || slotDuration%time.Millisecond != 0 {
		return 0, fmt.Errorf("grid: slot duration %v must be a positive whole number of milliseconds", slotDuration)
	}
	unixMS := t.UnixMilli()
	if unixMS < 0 {
		return 0, fmt.Errorf("grid: time %v is before the Unix epoch", t)
	}
	return unixMS / ms, nil
}
