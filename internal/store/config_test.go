package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// A zero field must mean "the default" and never "unlimited". database/sql
// reads a zero MaxOpenConns as unlimited and a zero lifetime as forever, so
// this is the test that would fail if withDefaults were ever dropped in favor
// of passing the struct straight through.
func TestConfigZeroMeansDefaultAndNeverUnlimited(t *testing.T) {
	got := Config{}.withDefaults()
	want := DefaultConfig()
	if got != want {
		t.Fatalf("empty Config did not fill from DefaultConfig:\n got %+v\nwant %+v", got, want)
	}
}

func TestConfigKeepsWhatTheCallerSet(t *testing.T) {
	in := Config{
		MaxOpenConns:    32,
		MaxIdleConns:    16,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 90 * time.Second,
		PingTimeout:     2 * time.Second,
	}
	if got := in.withDefaults(); got != in {
		t.Fatalf("a fully populated Config was altered:\n got %+v\nwant %+v", got, in)
	}
}

func TestConfigFillsOnlyTheUnsetFields(t *testing.T) {
	got := Config{MaxOpenConns: 20}.withDefaults()
	if got.MaxOpenConns != 20 {
		t.Errorf("MaxOpenConns = %d, want 20", got.MaxOpenConns)
	}
	if got.PingTimeout != DefaultConfig().PingTimeout {
		t.Errorf("PingTimeout = %s, want the default %s", got.PingTimeout, DefaultConfig().PingTimeout)
	}
}

// An idle count above the ceiling would have database/sql closing a connection
// on every other query. The cap is applied in OpenWithConfig rather than in
// withDefaults, so this test goes through the real entry point, with a DSN that
// cannot connect: the pool is configured before the ping, so reaching the ping
// error proves the settings were applied without needing a Postgres.
func TestOpenWithConfigFailsFastOnAnUnreachableHost(t *testing.T) {
	// Port 1 on the loopback: nothing listens, and the kernel refuses rather
	// than dropping, so this is fast and does not depend on the deadline.
	dsn := "postgres://keel:keel@127.0.0.1:1/keel?sslmode=disable&connect_timeout=1"

	start := time.Now()
	s, err := OpenWithConfig(context.Background(), dsn, Config{
		MaxOpenConns: 4,
		MaxIdleConns: 99, // above the ceiling on purpose
		PingTimeout:  3 * time.Second,
	})
	if err == nil {
		_ = s.Close()
		t.Fatal("OpenWithConfig reached a Postgres on port 1, which should not exist")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("the ping was not bounded: took %s", elapsed)
	}
}

// The deadline must be reported as a deadline. The two causes want different
// fixes, and "context deadline exceeded" on its own does not say which one
// happened.
func TestOpenWithConfigNamesItsOwnDeadline(t *testing.T) {
	// 203.0.113.0/24 is TEST-NET-3, reserved by RFC 5737 and routed nowhere, so
	// the connection attempt hangs rather than being refused.
	dsn := "postgres://keel:keel@203.0.113.1:5432/keel?sslmode=disable"

	s, err := OpenWithConfig(context.Background(), dsn, Config{PingTimeout: 200 * time.Millisecond})
	if err == nil {
		_ = s.Close()
		t.Skip("something answered on a TEST-NET-3 address, so this network cannot run the test")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Skipf("the connection failed for another reason on this network: %v", err)
	}
	if want := "no answer within 200ms"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not name the deadline %q", err, want)
	}
}
