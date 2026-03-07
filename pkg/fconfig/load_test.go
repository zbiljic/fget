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

func TestLoadEffectiveConfig_UsesNearestLinkConfig(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	cwd := filepath.Join(homeDir, "dev", "wtopic___", "fs___", "nested")

	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}

	homeConfigPath := filepath.Join(homeDir, "fget.yaml")
	if err := os.WriteFile(homeConfigPath, []byte("version: \"1\"\nroots:\n  - ~/dev/src\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(homeConfigPath) error = %v", err)
	}

	parentConfigPath := filepath.Join(homeDir, "dev", "fget.yaml")
	if err := os.MkdirAll(filepath.Dir(parentConfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(parentConfigPath) error = %v", err)
	}
	if err := os.WriteFile(parentConfigPath, []byte("version: \"1\"\nlink:\n  tags:\n    - parent\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(parentConfigPath) error = %v", err)
	}

	projectConfigPath := filepath.Join(homeDir, "dev", "wtopic___", "fs___", "fget.yaml")
	projectConfigContent := "" +
		"version: \"1\"\n" +
		"link:\n" +
		"  tags:\n" +
		"    - fs___\n" +
		"  root: .\n" +
		"  source_root: ~/dev/src\n"
	if err := os.MkdirAll(filepath.Dir(projectConfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(projectConfigPath) error = %v", err)
	}
	if err := os.WriteFile(projectConfigPath, []byte(projectConfigContent), 0o644); err != nil {
		t.Fatalf("WriteFile(projectConfigPath) error = %v", err)
	}

	eff, err := LoadEffectiveConfig(homeDir, cwd, "")
	if err != nil {
		t.Fatalf("LoadEffectiveConfig() error = %v", err)
	}

	if eff.Link == nil {
		t.Fatal("effective link config = nil, want non-nil")
	}

	wantTags := []string{"fs___"}
	if !reflect.DeepEqual(eff.Link.Tags, wantTags) {
		t.Fatalf("effective link tags = %v, want %v", eff.Link.Tags, wantTags)
	}

	wantRoot := filepath.Join(homeDir, "dev", "wtopic___", "fs___")
	if eff.Link.Root != wantRoot {
		t.Fatalf("effective link root = %q, want %q", eff.Link.Root, wantRoot)
	}

	wantSourceRoot := filepath.Join(homeDir, "dev", "src")
	if eff.Link.SourceRoot != wantSourceRoot {
		t.Fatalf("effective link source_root = %q, want %q", eff.Link.SourceRoot, wantSourceRoot)
	}
}
