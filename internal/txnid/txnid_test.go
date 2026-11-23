package txnid

import (
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

// bootTime is 2026-11-15T08:26:40Z, i.e. 1795000000000 ms.
var bootTime = time.UnixMilli(1795000000000)

func fixedClock(t time.Time) Clock { return func() time.Time { return t } }

func newGen(t *testing.T, id string, boot time.Time) *Generator {
	t.Helper()
	g, err := NewGenerator(id, fixedClock(boot))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestNextFormat(t *testing.T) {
	g := newGen(t, "coord-1", bootTime)
	for _, want := range []string{
		"coord-1-1795000000000-000001",
		"coord-1-1795000000000-000002",
		"coord-1-1795000000000-000003",
	} {
		if got := g.Next(); got != want {
			t.Errorf("Next() = %q, want %q", got, want)
		}
	}
	if g.CoordinatorID() != "coord-1" || g.BootUnixMS() != 1795000000000 {
		t.Errorf("CoordinatorID() = %q, BootUnixMS() = %d", g.CoordinatorID(), g.BootUnixMS())
	}
}

func TestBootTimeReadOnce(t *testing.T) {
	calls := 0
	clock := func() time.Time { calls++; return bootTime.Add(time.Duration(calls) * time.Hour) }
	g, err := NewGenerator("coord-1", clock)
	if err != nil {
		t.Fatal(err)
	}
	a, b := g.Next(), g.Next()
	if calls != 1 {
		t.Errorf("clock called %d times, want 1", calls)
	}
	if strings.TrimSuffix(a, "000001") != strings.TrimSuffix(b, "000002") {
		t.Errorf("boot time changed between IDs: %q, %q", a, b)
	}
}

func TestNilClockUsesNow(t *testing.T) {
	before := time.Now().UnixMilli()
	g, err := NewGenerator("coord-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if b := g.BootUnixMS(); b < before || b > time.Now().UnixMilli() {
		t.Errorf("BootUnixMS() = %d, not around now", b)
	}
}

func TestCounterPadding(t *testing.T) {
	tests := []struct {
		last uint64 // counter value before Next
		want string
	}{
		{122, "coord-1-1795000000000-000123"},
		{999998, "coord-1-1795000000000-999999"},
		{999999, "coord-1-1795000000000-1000000"}, // grows past six digits, never truncates
		{math.MaxUint64 - 1, "coord-1-1795000000000-18446744073709551615"},
	}
	for _, tt := range tests {
		g := newGen(t, "coord-1", bootTime)
		g.counter.Store(tt.last)
		if got := g.Next(); got != tt.want {
			t.Errorf("after %d: Next() = %q, want %q", tt.last, got, tt.want)
		}
	}
}

func TestNewGeneratorRejectsBadInput(t *testing.T) {
	bad := []string{"", "coord 1", "coord'1", "coord;drop", "cöord", strings.Repeat("a", MaxCoordinatorIDLen+1)}
	for _, id := range bad {
		if _, err := NewGenerator(id, fixedClock(bootTime)); err == nil {
			t.Errorf("NewGenerator(%q) succeeded, want error", id)
		}
	}
	for _, boot := range []time.Time{time.UnixMilli(0), time.UnixMilli(-1)} {
		if _, err := NewGenerator("coord-1", fixedClock(boot)); err == nil {
			t.Errorf("boot %v accepted, want error", boot)
		}
	}
	if _, err := NewGenerator(strings.Repeat("a", MaxCoordinatorIDLen), fixedClock(bootTime)); err != nil {
		t.Errorf("max-length id rejected: %v", err)
	}
}

func TestLongestGIDFitsPostgres(t *testing.T) {
	// Largest possible boot time and counter with the longest allowed ID.
	g := newGen(t, strings.Repeat("a", MaxCoordinatorIDLen), time.UnixMilli(math.MaxInt64))
	g.counter.Store(math.MaxUint64 - 1)
	gid := GID(g.Next())
	if len(gid) != MaxGIDLen {
		t.Errorf("worst-case GID length %d, want exactly %d", len(gid), MaxGIDLen)
	}
}

func TestConcurrentUniqueness(t *testing.T) {
	g := newGen(t, "coord-1", bootTime)
	const workers, perWorker = 16, 2000
	var mu sync.Mutex
	seen := make(map[string]struct{}, workers*perWorker)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]string, 0, perWorker)
			for range perWorker {
				local = append(local, g.Next())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				if _, dup := seen[id]; dup {
					t.Errorf("duplicate txn id %q", id)
				}
				seen[id] = struct{}{}
			}
		}()
	}
	wg.Wait()
	if len(seen) != workers*perWorker {
		t.Errorf("got %d unique ids, want %d", len(seen), workers*perWorker)
	}
}

// TestRestartNeverCollides simulates a coordinator restart: same ID, new boot
// time, counter back at 1.
func TestRestartNeverCollides(t *testing.T) {
	const n = 10000
	first := newGen(t, "coord-1", bootTime)
	second := newGen(t, "coord-1", bootTime.Add(time.Millisecond))
	seen := make(map[string]bool, 2*n)
	for range n {
		seen[first.Next()] = true
	}
	for range n {
		if id := second.Next(); seen[id] {
			t.Fatalf("restarted generator reused %q", id)
		}
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		txn  string
		want Parts
		ok   bool
	}{
		{"coord-1-1795000000000-000123", Parts{"coord-1", 1795000000000, 123}, true},
		{"c-1-1795000000000-1000000", Parts{"c-1", 1795000000000, 1000000}, true},
		{"coord_7-1-000001", Parts{"coord_7", 1, 1}, true},
		{"coord-1-000123", Parts{"coord", 1, 123}, true}, // old format is ambiguous; parses with id "coord"
		{"coord-1795000000000-000123", Parts{"coord", 1795000000000, 123}, true},
		{"1795000000000-000123", Parts{}, false},         // no coordinator id
		{"coord-1-1795000000000-123", Parts{}, false},    // counter not padded
		{"coord-1-1795000000000-000000", Parts{}, false}, // counter starts at 1
		{"coord-1-0-000001", Parts{}, false},             // boot before epoch
		{"coord-1-+179500000-000001", Parts{}, false},    // sign
		{"coord-1-1795000000000-00012a", Parts{}, false}, // not digits
		{"coord-1-1795000000000-", Parts{}, false},
		{"verify", Parts{}, false},
		{"", Parts{}, false},
	}
	for _, tt := range tests {
		got, ok := Parse(tt.txn)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Parse(%q) = %+v, %v; want %+v, %v", tt.txn, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParseRoundTrip(t *testing.T) {
	g := newGen(t, "coord-12-east", bootTime)
	for i := uint64(1); i <= 3; i++ {
		p, ok := Parse(g.Next())
		if !ok || p != (Parts{"coord-12-east", 1795000000000, i}) {
			t.Errorf("round trip %d: %+v, %v", i, p, ok)
		}
	}
}

func TestGIDRoundTrip(t *testing.T) {
	const txn = "coord-1-1795000000000-000123"
	gid := GID(txn)
	if gid != "skygrid:coord-1-1795000000000-000123" {
		t.Fatalf("GID() = %q", gid)
	}
	got, ok := ParseGID(gid)
	if !ok || got != txn {
		t.Errorf("ParseGID(%q) = %q, %v", gid, got, ok)
	}
}

func TestParseGID(t *testing.T) {
	tests := []struct {
		gid     string
		wantTxn string
		wantOK  bool
	}{
		{"skygrid:coord-1-1795000000000-000001", "coord-1-1795000000000-000001", true},
		{"skygrid:verify", "verify", true},
		{"skygrid:", "", false},
		{"", "", false},
		{"other:coord-1-1795000000000-000001", "", false},
		{"SKYGRID:coord-1-1795000000000-000001", "", false},
		{"xskygrid:coord-1", "", false},
		{"coord-1-1795000000000-000001", "", false},
	}
	for _, tt := range tests {
		txn, ok := ParseGID(tt.gid)
		if txn != tt.wantTxn || ok != tt.wantOK {
			t.Errorf("ParseGID(%q) = %q, %v; want %q, %v", tt.gid, txn, ok, tt.wantTxn, tt.wantOK)
		}
		if IsSkyGridGID(tt.gid) != tt.wantOK {
			t.Errorf("IsSkyGridGID(%q) = %v, want %v", tt.gid, !tt.wantOK, tt.wantOK)
		}
	}
}
