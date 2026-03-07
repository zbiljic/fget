package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestShouldSilenceExecuteError(t *testing.T) {
	t.Parallel()

	if !shouldSilenceExecuteError(context.Canceled) {
		t.Fatal("context.Canceled should be silenced")
	}

	if !shouldSilenceExecuteError(fmt.Errorf("wrapped: %w", context.Canceled)) {
		t.Fatal("wrapped context.Canceled should be silenced")
	}

	if !shouldSilenceExecuteError(errors.New(context.Canceled.Error())) {
		t.Fatal("plain context.Canceled message should be silenced")
	}

	if shouldSilenceExecuteError(errors.New("boom")) {
		t.Fatal("non-cancel error should not be silenced")
	}
}
