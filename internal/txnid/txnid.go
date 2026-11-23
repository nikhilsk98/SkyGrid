// Package txnid generates transaction IDs and maps them to Postgres
// prepared-transaction GIDs, as specified in docs/design.md section 3.
//
// A txn ID is "<coordinator_id>-<boot_unix_ms>-<counter>". The boot time is
// captured once per generator, so a restarted coordinator never reuses an ID
// from a previous run even though its counter starts again at 1.
package txnid

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// GIDPrefix marks every prepared transaction owned by SkyGrid. The recovery
// service only touches GIDs that start with it.
const GIDPrefix = "skygrid:"

// counterWidth is the minimum number of digits in the counter part of a txn ID.
const counterWidth = 6

// MaxGIDLen is the longest GID Postgres accepts (GIDSIZE 200, including the
// trailing NUL byte).
const MaxGIDLen = 199

const (
	maxBootDigits    = 19 // digits in the largest int64 (boot_unix_ms)
	maxCounterDigits = 20 // digits in the largest uint64 (counter)
)

// MaxCoordinatorIDLen is the longest coordinator ID whose GIDs are guaranteed
// to fit in Postgres:
// "skygrid:" + id + "-" + boot_unix_ms (<= 19 digits) + "-" + counter (<= 20 digits).
const MaxCoordinatorIDLen = MaxGIDLen - len(GIDPrefix) - 1 - maxBootDigits - 1 - maxCounterDigits

// ValidateCoordinatorID reports whether id can be embedded in txn IDs and in
// PREPARE TRANSACTION statements. Only ASCII letters, digits, '-' and '_' are
// allowed, so a GID never needs quoting or escaping.
func ValidateCoordinatorID(id string) error {
	if id == "" {
		return errors.New("coordinator id must not be empty")
	}
	if len(id) > MaxCoordinatorIDLen {
		return fmt.Errorf("coordinator id %q is longer than %d characters", id, MaxCoordinatorIDLen)
	}
	for _, r := range id {
		if !isIDRune(r) {
			return fmt.Errorf("coordinator id %q contains %q; only letters, digits, '-' and '_' are allowed", id, r)
		}
	}
	return nil
}

func isIDRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
}

// Clock returns the current time. Production code passes time.Now; tests
// pass a fixed clock.
type Clock func() time.Time

// Generator produces txn IDs of the form
// "<coordinatorID>-<bootUnixMS>-<counter>", with the counter zero-padded to
// at least six digits and starting at 1. It is safe for concurrent use.
type Generator struct {
	prefix     string // "<coordinatorID>-<bootUnixMS>-"
	coordID    string
	bootUnixMS int64
	counter    atomic.Uint64
}

// NewGenerator returns a Generator for coordinatorID whose boot time is read
// once from clock. A nil clock means time.Now.
func NewGenerator(coordinatorID string, clock Clock) (*Generator, error) {
	if err := ValidateCoordinatorID(coordinatorID); err != nil {
		return nil, fmt.Errorf("txnid: %w", err)
	}
	if clock == nil {
		clock = time.Now
	}
	boot := clock().UnixMilli()
	if boot <= 0 {
		return nil, fmt.Errorf("txnid: boot time %d ms is not after the Unix epoch", boot)
	}
	return &Generator{
		prefix:     coordinatorID + "-" + strconv.FormatInt(boot, 10) + "-",
		coordID:    coordinatorID,
		bootUnixMS: boot,
	}, nil
}

// CoordinatorID returns the coordinator ID embedded in every generated txn ID.
func (g *Generator) CoordinatorID() string { return g.coordID }

// BootUnixMS returns the boot time embedded in every generated txn ID.
func (g *Generator) BootUnixMS() int64 { return g.bootUnixMS }

// Next returns a new, unique txn ID.
func (g *Generator) Next() string {
	return fmt.Sprintf("%s%0*d", g.prefix, counterWidth, g.counter.Add(1))
}

// Parts are the components of a txn ID.
type Parts struct {
	CoordinatorID string
	BootUnixMS    int64
	Counter       uint64
}

// Parse splits a txn ID into its parts. Because the boot time and counter
// never contain '-', it splits from the right, so coordinator IDs may
// themselves contain '-'.
func Parse(txnID string) (Parts, bool) {
	rest, counterStr, ok := cutLast(txnID)
	if !ok || len(counterStr) < counterWidth {
		return Parts{}, false
	}
	coordID, bootStr, ok := cutLast(rest)
	if !ok || ValidateCoordinatorID(coordID) != nil {
		return Parts{}, false
	}
	boot, err := strconv.ParseInt(bootStr, 10, 64)
	if err != nil || boot <= 0 || !allDigits(bootStr) {
		return Parts{}, false
	}
	counter, err := strconv.ParseUint(counterStr, 10, 64)
	if err != nil || counter == 0 || !allDigits(counterStr) {
		return Parts{}, false
	}
	return Parts{CoordinatorID: coordID, BootUnixMS: boot, Counter: counter}, true
}

func cutLast(s string) (before, after string, ok bool) {
	i := strings.LastIndexByte(s, '-')
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// GID returns the Postgres prepared-transaction GID for txnID.
func GID(txnID string) string {
	return GIDPrefix + txnID
}

// ParseGID extracts the txn ID from a SkyGrid GID. ok is false when gid does
// not carry the SkyGrid prefix or has nothing after it. It does not require
// the txn ID to be well formed, so recovery can still resolve any skygrid:*
// GID (including the "skygrid:verify" smoke test).
func ParseGID(gid string) (txnID string, ok bool) {
	txnID, ok = strings.CutPrefix(gid, GIDPrefix)
	if !ok || txnID == "" {
		return "", false
	}
	return txnID, true
}

// IsSkyGridGID reports whether gid belongs to SkyGrid and may be resolved by
// the recovery service.
func IsSkyGridGID(gid string) bool {
	_, ok := ParseGID(gid)
	return ok
}
