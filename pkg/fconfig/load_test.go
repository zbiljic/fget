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

func TestLoadEffectiveConfig_ResolvesRootsRelativeToDeclaringFile(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	cwd := filepath.Join(homeDir, "dev", "proj", "nested")

	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}

	configPath := filepath.Join(homeDir, "dev", "fget.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(configDir) error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte("version: \"1\"\nroots:\n  - ./src\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(configPath) error = %v", err)
	}

	eff, err := LoadEffectiveConfig(homeDir, cwd, "")
	if err != nil {
		t.Fatalf("LoadEffectiveConfig() error = %v", err)
	}

	wantRoots := []string{filepath.Join(homeDir, "dev", "src")}
	if !reflect.DeepEqual(eff.Roots, wantRoots) {
		t.Fatalf("effective roots = %v, want %v", eff.Roots, wantRoots)
	}
}

func TestLoadConfigFile_ResolvesCatalogImportsToConfigFiles(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	scopeDir := filepath.Join(homeDir, "dev", "scope")
	configPath := filepath.Join(scopeDir, "fget.yaml")

	if err := os.MkdirAll(scopeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(scopeDir) error = %v", err)
	}
	content := "" +
		"version: \"2\"\n" +
		"catalog:\n" +
		"  path: ./fget.catalog.yaml\n" +
		"  imports:\n" +
		"    - ../drive-a\n" +
		"    - ../drive-b/fget.yaml\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(configPath) error = %v", err)
	}

	cfg, err := LoadConfigFile(configPath, homeDir)
	if err != nil {
		t.Fatalf("LoadConfigFile() error = %v", err)
	}

	want := []string{
		filepath.Join(homeDir, "dev", "drive-a", "fget.yaml"),
		filepath.Join(homeDir, "dev", "drive-b", "fget.yaml"),
	}
	if !reflect.DeepEqual(cfg.Catalog.Imports, want) {
		t.Fatalf("catalog.imports = %v, want %v", cfg.Catalog.Imports, want)
	}
}

func TestLoadEffectiveConfig_V2ScopeOwnerIsolatesOuterConfigs(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	xdgConfigHome := filepath.Join(homeDir, "xdg")
	baseConfigPath := ResolveBaseConfigPath(xdgConfigHome, homeDir)

	if err := os.MkdirAll(filepath.Dir(baseConfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(baseConfigDir) error = %v", err)
	}
	baseConfig := "" +
		"version: \"2\"\n" +
		"roots:\n" +
		"  - ~/dev/base\n" +
		"catalog:\n" +
		"  path: ~/catalogs/home.yaml\n"
	if err := os.WriteFile(baseConfigPath, []byte(baseConfig), 0o644); err != nil {
		t.Fatalf("WriteFile(baseConfigPath) error = %v", err)
	}

	scopeRoot := t.TempDir()
	cwd := filepath.Join(scopeRoot, "proj", "nested")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}

	scopeConfigPath := filepath.Join(scopeRoot, "fget.yaml")
	scopeConfig := "" +
		"version: \"2\"\n" +
		"roots:\n" +
		"  - ./src\n" +
		"catalog:\n" +
		"  path: ./fget.catalog.yaml\n"
	if err := os.WriteFile(scopeConfigPath, []byte(scopeConfig), 0o644); err != nil {
		t.Fatalf("WriteFile(scopeConfigPath) error = %v", err)
	}

	projectConfigPath := filepath.Join(scopeRoot, "proj", "fget.yaml")
	projectConfig := "" +
		"version: \"2\"\n" +
		"roots:\n" +
		"  - ./nested-src\n" +
		"link:\n" +
		"  tags:\n" +
		"    - ext\n"
	if err := os.WriteFile(projectConfigPath, []byte(projectConfig), 0o644); err != nil {
		t.Fatalf("WriteFile(projectConfigPath) error = %v", err)
	}

	eff, err := LoadEffectiveConfig(homeDir, cwd, xdgConfigHome)
	if err != nil {
		t.Fatalf("LoadEffectiveConfig() error = %v", err)
	}

	wantRoots := []string{
		filepath.Join(scopeRoot, "src"),
		filepath.Join(scopeRoot, "proj", "nested-src"),
	}
	if !reflect.DeepEqual(eff.Roots, wantRoots) {
		t.Fatalf("effective roots = %v, want %v", eff.Roots, wantRoots)
	}

	wantSources := []string{scopeConfigPath, projectConfigPath}
	if !reflect.DeepEqual(eff.Sources, wantSources) {
		t.Fatalf("effective sources = %v, want %v", eff.Sources, wantSources)
	}

	wantCatalogPath := filepath.Join(scopeRoot, "fget.catalog.yaml")
	if eff.Catalog.Path != wantCatalogPath {
		t.Fatalf("effective catalog path = %q, want %q", eff.Catalog.Path, wantCatalogPath)
	}
	if eff.ScopeOwner != scopeConfigPath {
		t.Fatalf("effective scope owner = %q, want %q", eff.ScopeOwner, scopeConfigPath)
	}
	if containsString(eff.Roots, filepath.Join(homeDir, "dev", "base")) {
		t.Fatalf("effective roots unexpectedly include base scope roots: %v", eff.Roots)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
