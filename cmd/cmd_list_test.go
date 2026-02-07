package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-git/go-git/v5"
)

func TestIsListSkippableRepoError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "remote not found",
			err:  git.ErrRemoteNotFound,
			want: true,
		},
		{
			name: "wrapped remote not found",
			err:  fmt.Errorf("prefix: %w", git.ErrRemoteNotFound),
			want: true,
		},
		{
			name: "same message different error",
			err:  errors.New(git.ErrRemoteNotFound.Error()),
			want: false,
		},
		{
			name: "other error",
			err:  git.ErrRepositoryNotExists,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isListSkippableRepoError(tt.err)
			if got != tt.want {
				t.Fatalf("isListSkippableRepoError() = %v, want %v", got, tt.want)
			}
		})
	}
}
