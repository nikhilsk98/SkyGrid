package grid

import (
	"testing"
	"time"
)

func TestSlotAt(t *testing.T) {
	tests := []struct {
		unixMS int64
		dur    time.Duration
		want   int64
	}{
		{1795000000000, time.Second, 1795000000},
		{1795000000999, time.Second, 1795000000}, // integer division, not rounding
		{1795000001000, time.Second, 1795000001},
		{1795000000250, 250 * time.Millisecond, 7180000001},
		{0, time.Second, 0},
	}
	for _, tt := range tests {
		got, err := SlotAt(time.UnixMilli(tt.unixMS), tt.dur)
		if err != nil || got != tt.want {
			t.Errorf("SlotAt(%d ms, %v) = %d, %v; want %d", tt.unixMS, tt.dur, got, err, tt.want)
		}
	}
}

func TestSlotAtRejectsBadInput(t *testing.T) {
	now := time.UnixMilli(1795000000000)
	for _, d := range []time.Duration{0, -time.Second, 1500 * time.Microsecond} {
		if _, err := SlotAt(now, d); err == nil {
			t.Errorf("SlotAt(_, %v) succeeded, want error", d)
		}
	}
	if _, err := SlotAt(time.UnixMilli(-1), time.Second); err == nil {
		t.Error("pre-epoch time accepted")
	}
}
