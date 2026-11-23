package db

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewPoolRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	if _, err := NewPool(ctx, "", 1); !errors.Is(err, ErrEmptyDSN) {
		t.Errorf("empty dsn: err = %v, want ErrEmptyDSN", err)
	}
	if _, err := NewPool(ctx, "postgres://u:p@localhost/db", 0); err == nil {
		t.Error("maxConns 0 accepted")
	}
	_, err := NewPool(ctx, "postgres://u:topsecret@localhost:notaport/db", 1)
	if err == nil {
		t.Fatal("malformed dsn accepted")
	}
	if strings.Contains(err.Error(), "topsecret") {
		t.Errorf("error leaks password: %v", err)
	}
}

func TestNewPoolUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Port 1 on loopback refuses connections immediately.
	_, err := NewPool(ctx, "postgres://u:topsecret@127.0.0.1:1/db?connect_timeout=1", 1)
	if err == nil {
		t.Fatal("unreachable database accepted")
	}
	if !strings.Contains(err.Error(), "ping 127.0.0.1:1/db") {
		t.Errorf("error does not name the target: %v", err)
	}
	if strings.Contains(err.Error(), "topsecret") {
		t.Errorf("error leaks password: %v", err)
	}
}

// TestNewPoolLive runs against a real database when SKYGRID_TEST_DSN is set,
// e.g. SKYGRID_TEST_DSN=postgres://skygrid:skygrid@localhost:5450/skygrid
func TestNewPoolLive(t *testing.T) {
	dsn := os.Getenv("SKYGRID_TEST_DSN")
	if dsn == "" {
		t.Skip("SKYGRID_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := NewPool(ctx, dsn, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if got := pool.Config().MaxConns; got != 2 {
		t.Errorf("MaxConns = %d, want 2", got)
	}
	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Errorf("SELECT 1 = %d, %v", one, err)
	}
}
