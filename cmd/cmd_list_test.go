package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

func TestNormalizeStateFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to all", input: "", want: stateFilterAll},
		{name: "all", input: "all", want: stateFilterAll},
		{name: "active", input: "active", want: stateFilterActive},
		{name: "inactive", input: "inactive", want: stateFilterInactive},
		{name: "archived alias", input: "archived", want: stateFilterInactive},
		{name: "invalid", input: "foo", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeStateFilter(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeStateFilter() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Fatalf("normalizeStateFilter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShouldIncludeByState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter string
		active bool
		want   bool
	}{
		{name: "all includes active", filter: stateFilterAll, active: true, want: true},
		{name: "all includes inactive", filter: stateFilterAll, active: false, want: true},
		{name: "active includes active", filter: stateFilterActive, active: true, want: true},
		{name: "active excludes inactive", filter: stateFilterActive, active: false, want: false},
		{name: "inactive excludes active", filter: stateFilterInactive, active: true, want: false},
		{name: "inactive includes inactive", filter: stateFilterInactive, active: false, want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := shouldIncludeByState(tt.filter, tt.active)
			if got != tt.want {
				t.Fatalf("shouldIncludeByState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutputTableStateColumn(t *testing.T) {
	t.Parallel()

	now := time.Now()
	active := true
	inactive := false
	repos := []repoInfo{
		{
			Path:        "github.com/example/one",
			URL:         "https://github.com/example/one.git",
			Branch:      "main",
			IsClean:     true,
			LastUpdated: now,
			CommitCount: 3,
			Active:      &active,
		},
		{
			Path:        "github.com/example/two",
			URL:         "https://github.com/example/two.git",
			Branch:      "master",
			IsClean:     false,
			LastUpdated: now,
			CommitCount: 7,
			Active:      &inactive,
		},
	}

	var withState strings.Builder
	if err := outputTable(&withState, repos, true); err != nil {
		t.Fatalf("outputTable(with state) error = %v", err)
	}
	if !strings.Contains(withState.String(), "State") {
		t.Fatalf("outputTable(with state) does not contain State header:\n%s", withState.String())
	}
	if !strings.Contains(withState.String(), repoStateInactive) {
		t.Fatalf("outputTable(with state) does not contain %q value:\n%s", repoStateInactive, withState.String())
	}

	var withoutState strings.Builder
	if err := outputTable(&withoutState, repos, false); err != nil {
		t.Fatalf("outputTable(without state) error = %v", err)
	}
	if strings.Contains(withoutState.String(), "State") {
		t.Fatalf("outputTable(without state) should not contain State header:\n%s", withoutState.String())
	}
}
