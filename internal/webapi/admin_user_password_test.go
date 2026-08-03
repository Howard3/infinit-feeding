package webapi

import (
	"errors"
	"strings"
	"testing"
)

func TestGenerateTempPassword_Length(t *testing.T) {
	password, err := generateTempPassword()
	if err != nil {
		t.Fatalf("generateTempPassword() error: %v", err)
	}
	if len(password) != tempPasswordLength {
		t.Fatalf("expected length %d, got %d", tempPasswordLength, len(password))
	}
}

func TestGenerateTempPassword_Charset(t *testing.T) {
	password, err := generateTempPassword()
	if err != nil {
		t.Fatalf("generateTempPassword() error: %v", err)
	}
	for i, c := range password {
		if !strings.ContainsRune(tempPasswordCharset, c) {
			t.Fatalf("character %q at index %d is not in allowed charset", c, i)
		}
	}
}

func TestGenerateTempPassword_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 50)
	for i := 0; i < 50; i++ {
		password, err := generateTempPassword()
		if err != nil {
			t.Fatalf("generateTempPassword() error: %v", err)
		}
		if _, exists := seen[password]; exists {
			t.Fatalf("duplicate password generated: %q", password)
		}
		seen[password] = struct{}{}
	}
}

func TestSetUserPasswordWithRetry_SucceedsAfterFailures(t *testing.T) {
	attempts := 0
	password, err := setUserPasswordWithRetry("user_123", func(userID, password string) error {
		attempts++
		if attempts < 3 {
			return errors.New("password rejected")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if password == "" {
		t.Fatal("expected a password")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestSetUserPasswordWithRetry_ExhaustsAttempts(t *testing.T) {
	attempts := 0
	_, err := setUserPasswordWithRetry("user_123", func(userID, password string) error {
		attempts++
		return errors.New("password rejected")
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != passwordResetMaxAttempts {
		t.Fatalf("expected %d attempts, got %d", passwordResetMaxAttempts, attempts)
	}
}
