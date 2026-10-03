package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestIsBusyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "unrelated", err: errors.New("no such table: schools"), want: false},
		{name: "sqlite locked", err: errors.New("SQLite failure: `database is locked`"), want: true},
		{name: "wrapped locked", err: fmt.Errorf("failed to list schools: %w", errors.New("database is locked")), want: true},
		{name: "SQLITE_BUSY", err: errors.New("SQLITE_BUSY: database is locked"), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBusyError(tc.err); got != tc.want {
				t.Fatalf("IsBusyError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestRetryTransientRetriesBusy(t *testing.T) {
	attempts := 0
	err := RetryTransient(context.Background(), 4, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("SQLite failure: `database is locked`")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected busy retries to succeed, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestRetryTransientHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RetryTransient(ctx, 4, func() error {
		return errors.New("database is locked")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestRetryTransientSkipsSleepAfterLastAttempt(t *testing.T) {
	start := time.Now()
	err := RetryTransient(context.Background(), 2, func() error {
		return errors.New("database is locked")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// One sleep between attempts (~1.2s); must not sleep again after the last.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("elapsed %v suggests sleep after last attempt", elapsed)
	}
}
