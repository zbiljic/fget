package fconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadEffectiveConfig_MergesHomeToCwd(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	xdgConfigHome := filepath.Join(homeDir, "xdg")
	cwd := filepath.Join(homeDir, "dev", "proj", "sub")

	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}

	baseConfigPath := ResolveBaseConfigPath(xdgConfigHome, homeDir)
	if err := os.MkdirAll(filepath.Dir(baseConfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(baseConfigDir) error = %v", err)
	}
	if err := os.WriteFile(baseConfigPath, []byte("version: \"1\"\nroots:\n  - /a\n  - /b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(baseConfigPath) error = %v", err)
	}

	homeOverlayPath := filepath.Join(homeDir, "fget.yaml")
	if err := os.WriteFile(homeOverlayPath, []byte("version: \"1\"\nroots:\n  - /b\n  - /c\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(homeOverlayPath) error = %v", err)
	}

	projectOverlayPath := filepath.Join(homeDir, "dev", "proj", "fget.yaml")
	projectOverlayContent := "version: \"1\"\nroots:\n  - /d\ncatalog:\n  path: \"~/custom/catalog.yaml\"\n"
	if err := os.WriteFile(projectOverlayPath, []byte(projectOverlayContent), 0o644); err != nil {
		t.Fatalf("WriteFile(projectOverlayPath) error = %v", err)
	}

	eff, err := LoadEffectiveConfig(homeDir, cwd, xdgConfigHome)
	if err != nil {
		t.Fatalf("LoadEffectiveConfig() error = %v", err)
	}

	wantRoots := []string{"/a", "/b", "/c", "/d"}
	if !reflect.DeepEqual(eff.Roots, wantRoots) {
		t.Fatalf("effective roots = %v, want %v", eff.Roots, wantRoots)
	}

	wantSources := []string{baseConfigPath, homeOverlayPath, projectOverlayPath}
	if !reflect.DeepEqual(eff.Sources, wantSources) {
		t.Fatalf("effective sources = %v, want %v", eff.Sources, wantSources)
	}

	wantCatalogPath := filepath.Join(homeDir, "custom", "catalog.yaml")
	if eff.Catalog.Path != wantCatalogPath {
		t.Fatalf("effective catalog path = %q, want %q", eff.Catalog.Path, wantCatalogPath)
	}
}

func TestLoadEffectiveConfig_DefaultCatalogPath(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	cwd := filepath.Join(homeDir, "dev")

	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}

	overlayPath := filepath.Join(homeDir, "fget.yaml")
	if err := os.WriteFile(overlayPath, []byte("version: \"1\"\nroots:\n  - /a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(overlayPath) error = %v", err)
	}

	eff, err := LoadEffectiveConfig(homeDir, cwd, "")
	if err != nil {
		t.Fatalf("LoadEffectiveConfig() error = %v", err)
	}

	want := ResolveDefaultCatalogPath("", homeDir)
	if eff.Catalog.Path != want {
		t.Fatalf("effective catalog path = %q, want %q", eff.Catalog.Path, want)
	}
}
