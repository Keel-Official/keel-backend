package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/store"
)

// Unset means the default, and the default is store's, not a second copy of the
// numbers written out here. A test that restated them would pass while the two
// sides drifted.
func TestStoreConfigFromEnvIsTheStoreDefaultWhenNothingIsSet(t *testing.T) {
	got, err := storeConfigFromEnv()
	if err != nil {
		t.Fatalf("storeConfigFromEnv with a clean environment: %v", err)
	}
	if want := store.DefaultConfig(); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestStoreConfigFromEnvReadsEveryVariable(t *testing.T) {
	t.Setenv(envDBMaxOpenConns, "20")
	t.Setenv(envDBMaxIdleConns, "10")
	t.Setenv(envDBConnMaxLifetime, "1h")
	t.Setenv(envDBConnMaxIdleTime, "45s")
	t.Setenv(envDBPingTimeout, "2s")

	got, err := storeConfigFromEnv()
	if err != nil {
		t.Fatalf("storeConfigFromEnv: %v", err)
	}
	want := store.Config{
		MaxOpenConns:    20,
		MaxIdleConns:    10,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 45 * time.Second,
		PingTimeout:     2 * time.Second,
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// A value that cannot be parsed, or that is zero or negative, must REFUSE
// rather than fall back. A pool silently the wrong size, with a .env file
// saying it is the right one, is the failure these variables were added to make
// impossible.
func TestStoreConfigFromEnvRefusesNonsense(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{"a count that is not a number", envDBMaxOpenConns, "eight"},
		{"a count that is zero", envDBMaxOpenConns, "0"},
		{"a count that is negative", envDBMaxIdleConns, "-1"},
		{"a duration with no unit", envDBConnMaxLifetime, "30"},
		{"a duration that is not one", envDBPingTimeout, "soon"},
		{"a duration that is zero", envDBPingTimeout, "0s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			_, err := storeConfigFromEnv()
			if err == nil {
				t.Fatalf("%s=%q was accepted", tc.key, tc.value)
			}
			// The message has to name the variable, because the person reading
			// it is looking at a container that will not start.
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("error %q does not name %s", err, tc.key)
			}
		})
	}
}

// Empty is how docker-compose.prod.yml passes a variable that is not in .env:
// `${KEEL_DB_PING_TIMEOUT:-}`. It has to read as unset, or every deployment
// that sets none of the five would refuse to start.
func TestStoreConfigFromEnvTreatsEmptyAsUnset(t *testing.T) {
	t.Setenv(envDBMaxOpenConns, "")
	t.Setenv(envDBPingTimeout, "")

	got, err := storeConfigFromEnv()
	if err != nil {
		t.Fatalf("an empty value was rejected: %v", err)
	}
	if want := store.DefaultConfig(); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestEnvOrPrefersTheEnvironment(t *testing.T) {
	if got := envOr(envDSN, "fallback"); got != "fallback" {
		t.Errorf("unset: got %q, want the fallback", got)
	}
	t.Setenv(envDSN, "postgres://set")
	if got := envOr(envDSN, "fallback"); got != "postgres://set" {
		t.Errorf("set: got %q", got)
	}
}
