package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fconfig"
	"github.com/zbiljic/fget/pkg/vconfig"
)

func TestRunConfigSync_DoesNotSaveCatalogOnInspectionError(t *testing.T) {
	tmp := t.TempDir()
	scanRoot := filepath.Join(tmp, "repos")
	malformedRepo := filepath.Join(scanRoot, "malformed")
	if err := os.MkdirAll(filepath.Join(malformedRepo, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	catalogPath := filepath.Join(tmp, "fget.catalog.yaml")
	oldTimestamp := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)
	if err := fconfig.SaveCatalog(catalogPath, &fconfig.Catalog{
		Version: fconfig.CatalogVersionV1,
		Roots: []fconfig.CatalogRoot{
			{Path: scanRoot, LastScannedAt: oldTimestamp},
		},
		Repos: []fconfig.RepoEntry{
			{
				ID:        "github.com/acme/existing",
				RemoteURL: "https://example.com/acme/existing",
				Tags:      []string{"keep"},
				Locations: []fconfig.RepoLocation{
					{Path: filepath.Join(scanRoot, "existing"), LastSeenAt: oldTimestamp},
				},
			},
		},
	}); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}
	before, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("ReadFile(catalog before sync) error = %v", err)
	}

	if err := vconfig.SaveConfig(&fconfig.Config{
		Version: fconfig.ConfigVersionV2,
		Roots:   []string{scanRoot},
		Catalog: fconfig.CatalogConfig{Path: catalogPath},
	}, filepath.Join(tmp, "fget.yaml")); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWd)
	}()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir(%q) error = %v", tmp, err)
	}

	originalFlags := configSyncCmdFlags
	configSyncCmdFlags = configSyncOptions{
		Prune:   true,
		Silent:  true,
		Workers: 2,
	}
	defer func() {
		configSyncCmdFlags = originalFlags
	}()

	command := &cobra.Command{}
	command.SetContext(context.Background())
	err = runConfigSync(command, nil)
	if err == nil {
		t.Fatal("runConfigSync() error = nil, want inspection error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte(malformedRepo)) {
		t.Fatalf("runConfigSync() error = %q, want repository path %q", err, malformedRepo)
	}

	after, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("ReadFile(catalog after sync) error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("catalog file changed after inspection error:\n before: %s\n after: %s", before, after)
	}
}

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

	got, err := resolveSyncRoots(nil, nil, nil, nil, cwd, "home", func([]string, string) ([]string, error) {
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

	got, err := resolveSyncRoots([]string{"/flags"}, []string{"/args"}, []string{"/cfg"}, []string{"/imported/fget.yaml"}, "/cwd", "home", normalize)
	if err != nil {
		t.Fatalf("resolveSyncRoots() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"/flags"}) {
		t.Fatalf("flags precedence mismatch: %v", got)
	}

	got, err = resolveSyncRoots(nil, []string{"/args"}, []string{"/cfg"}, []string{"/imported/fget.yaml"}, "/cwd", "home", normalize)
	if err != nil {
		t.Fatalf("resolveSyncRoots() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"/args"}) {
		t.Fatalf("args precedence mismatch: %v", got)
	}

	got, err = resolveSyncRoots(nil, nil, []string{"/cfg"}, []string{"/imported/fget.yaml"}, "/cwd", "home", normalize)
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
	_, err := resolveSyncRoots([]string{"x"}, nil, nil, nil, "/cwd", "home", func([]string, string) ([]string, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("resolveSyncRoots() error = %v, want %v", err, wantErr)
	}
}

func TestResolveSyncRoots_RequiresExplicitRootsWhenImportsConfigured(t *testing.T) {
	t.Parallel()

	_, err := resolveSyncRoots(nil, nil, nil, []string{"/external/fget.yaml"}, "/home/user", "home", func([]string, string) ([]string, error) {
		t.Fatal("normalize should not be called when no local roots are configured")
		return nil, nil
	})
	if err == nil {
		t.Fatal("resolveSyncRoots() error = nil, want explicit roots error")
	}
	if got := err.Error(); got != "catalog sync requires explicit local roots when catalog imports are configured" {
		t.Fatalf("resolveSyncRoots() error = %q", got)
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
