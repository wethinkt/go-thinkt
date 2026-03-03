package thinkt

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrors_CanBeWrappedAndMatched(t *testing.T) {
	sentinels := []error{
		ErrSessionNotFound,
		ErrProjectNotFound,
		ErrSourceUnavailable,
		ErrResumeNotSupported,
	}
	for _, sentinel := range sentinels {
		wrapped := fmt.Errorf("context: %w", sentinel)
		if !errors.Is(wrapped, sentinel) {
			t.Errorf("errors.Is failed for wrapped %v", sentinel)
		}
	}
}

func TestValidationError_ImplementsError(t *testing.T) {
	err := &ValidationError{Field: "limit", Message: "must be positive"}
	if err.Error() != "invalid limit: must be positive" {
		t.Errorf("unexpected message: %s", err.Error())
	}

	var ve *ValidationError
	wrapped := fmt.Errorf("bad input: %w", err)
	if !errors.As(wrapped, &ve) {
		t.Fatal("errors.As failed for wrapped ValidationError")
	}
	if ve.Field != "limit" {
		t.Errorf("Field = %q, want %q", ve.Field, "limit")
	}
}
