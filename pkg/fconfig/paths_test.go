package fconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverConfigFiles_HomeToCwdOrder(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	workspaceRoot := filepath.Join(homeDir, "dev")
	cwd := filepath.Join(workspaceRoot, "proj", "sub")

	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}

	homeConfig := filepath.Join(homeDir, "fget.yaml")
	rootConfig := filepath.Join(workspaceRoot, "fget.yaml")
	projectConfig := filepath.Join(workspaceRoot, "proj", "fget.yaml")

	if err := os.WriteFile(homeConfig, []byte("version: \"1\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(homeConfig) error = %v", err)
	}
	if err := os.WriteFile(rootConfig, []byte("version: \"1\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(rootConfig) error = %v", err)
	}
	if err := os.WriteFile(projectConfig, []byte("version: \"1\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(projectConfig) error = %v", err)
	}

	got, err := DiscoverConfigFiles(cwd)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles() error = %v", err)
	}

	want := []string{homeConfig, rootConfig, projectConfig}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverConfigFiles() = %v, want %v", got, want)
	}
}

func TestDiscoverConfigFiles_OutsideHomeStillDiscovers(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	cwd := filepath.Join(workspaceRoot, "proj", "sub")

	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}

	rootConfig := filepath.Join(workspaceRoot, "fget.yaml")
	projectConfig := filepath.Join(workspaceRoot, "proj", "fget.yaml")

	if err := os.WriteFile(rootConfig, []byte("version: \"2\"\ncatalog:\n  path: ./fget.catalog.yaml\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(rootConfig) error = %v", err)
	}
	if err := os.WriteFile(projectConfig, []byte("version: \"2\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(projectConfig) error = %v", err)
	}

	got, err := DiscoverConfigFiles(cwd)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles() error = %v", err)
	}

	want := []string{rootConfig, projectConfig}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverConfigFiles() = %v, want %v", got, want)
	}
}

func TestResolveBaseConfigPath(t *testing.T) {
	t.Parallel()

	const home = "/Users/test"

	got := ResolveBaseConfigPath("", home)
	want := filepath.Join(home, ".config", "fget", "fget.yaml")
	if got != want {
		t.Fatalf("ResolveBaseConfigPath() = %q, want %q", got, want)
	}

	got = ResolveBaseConfigPath("/tmp/xdg", home)
	want = filepath.Join("/tmp/xdg", "fget", "fget.yaml")
	if got != want {
		t.Fatalf("ResolveBaseConfigPath() with xdg = %q, want %q", got, want)
	}
}

func TestResolveDefaultCatalogPath(t *testing.T) {
	t.Parallel()

	const home = "/Users/test"

	got := ResolveDefaultCatalogPath("", home)
	want := filepath.Join(home, ".config", "fget", "fget.catalog.yaml")
	if got != want {
		t.Fatalf("ResolveDefaultCatalogPath() = %q, want %q", got, want)
	}

	got = ResolveDefaultCatalogPath("/tmp/xdg", home)
	want = filepath.Join("/tmp/xdg", "fget", "fget.catalog.yaml")
	if got != want {
		t.Fatalf("ResolveDefaultCatalogPath() with xdg = %q, want %q", got, want)
	}
}
