package cmd

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseConfigSyncArgs(t *testing.T) {
	t.Parallel()

	dirA := t.TempDir()
	dirB := t.TempDir()

	got, err := parseConfigSyncArgs([]string{dirA, dirB})
	if err != nil {
		t.Fatalf("parseConfigSyncArgs() error = %v", err)
	}

	want := []string{filepath.Clean(dirA), filepath.Clean(dirB)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseConfigSyncArgs() = %v, want %v", got, want)
	}
}

func TestResolveSyncRoots_FallbackToCwd(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()

	got, err := resolveSyncRoots(nil, nil, nil, cwd, "home", func([]string, string) ([]string, error) {
		t.Fatal("normalize should not be called when no roots are configured")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("resolveSyncRoots() error = %v", err)
	}

	want := []string{cwd}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveSyncRoots() = %v, want %v", got, want)
	}
}

func TestResolveSyncRoots_Precedence(t *testing.T) {
	t.Parallel()

	normalize := func(roots []string, _ string) ([]string, error) {
		return append([]string{}, roots...), nil
	}

	got, err := resolveSyncRoots([]string{"/flags"}, []string{"/args"}, []string{"/cfg"}, "/cwd", "home", normalize)
	if err != nil {
		t.Fatalf("resolveSyncRoots() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"/flags"}) {
		t.Fatalf("flags precedence mismatch: %v", got)
	}

	got, err = resolveSyncRoots(nil, []string{"/args"}, []string{"/cfg"}, "/cwd", "home", normalize)
	if err != nil {
		t.Fatalf("resolveSyncRoots() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"/args"}) {
		t.Fatalf("args precedence mismatch: %v", got)
	}

	got, err = resolveSyncRoots(nil, nil, []string{"/cfg"}, "/cwd", "home", normalize)
	if err != nil {
		t.Fatalf("resolveSyncRoots() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"/cfg"}) {
		t.Fatalf("config precedence mismatch: %v", got)
	}
}

func TestResolveSyncRoots_PropagatesNormalizeError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	_, err := resolveSyncRoots([]string{"x"}, nil, nil, "/cwd", "home", func([]string, string) ([]string, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("resolveSyncRoots() error = %v, want %v", err, wantErr)
	}
}

func TestConfigSyncProgressText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		processed int
		total     int
		want      string
	}{
		{
			name:      "before discovery finishes",
			processed: 0,
			total:     0,
			want:      "finding repositories...",
		},
		{
			name:      "inspection started",
			processed: 0,
			total:     12,
			want:      "syncing catalog: 0/12",
		},
		{
			name:      "inspection complete",
			processed: 12,
			total:     12,
			want:      "syncing catalog: 12/12",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatConfigSyncProgressText(tt.processed, tt.total)
			if got != tt.want {
				t.Fatalf("formatConfigSyncProgressText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigSyncProgressEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		silent         bool
		interactive    bool
		wantProgressOn bool
	}{
		{
			name:           "silent disables progress",
			silent:         true,
			interactive:    true,
			wantProgressOn: false,
		},
		{
			name:           "interactive output enables progress",
			silent:         false,
			interactive:    true,
			wantProgressOn: true,
		},
		{
			name:           "non interactive output disables progress",
			silent:         false,
			interactive:    false,
			wantProgressOn: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := configSyncProgressEnabled(tt.silent, tt.interactive)
			if got != tt.wantProgressOn {
				t.Fatalf("configSyncProgressEnabled() = %v, want %v", got, tt.wantProgressOn)
			}
		})
	}
}
