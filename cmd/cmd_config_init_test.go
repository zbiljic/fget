package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zbiljic/fget/pkg/fconfig"
	"github.com/zbiljic/fget/pkg/vconfig"
)

func TestResolveInitTargetPath_FileFlag(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	home := filepath.Join(cwd, "home")

	got, err := resolveInitTargetPath(false, "~/configs/fget.yaml", cwd, home, "")
	if err != nil {
		t.Fatalf("resolveInitTargetPath() error = %v", err)
	}

	want := filepath.Join(home, "configs", "fget.yaml")
	if got != want {
		t.Fatalf("resolveInitTargetPath() = %q, want %q", got, want)
	}
}

func TestResolveInitTargetPath_LocalFlag(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()

	got, err := resolveInitTargetPath(true, "", cwd, "/home", "")
	if err != nil {
		t.Fatalf("resolveInitTargetPath() error = %v", err)
	}

	want := filepath.Join(cwd, "fget.yaml")
	if got != want {
		t.Fatalf("resolveInitTargetPath() = %q, want %q", got, want)
	}
}

func TestResolveInitTargetPath_DefaultGlobalPath(t *testing.T) {
	t.Parallel()

	got, err := resolveInitTargetPath(false, "", "/cwd", "/home/user", "/xdg")
	if err != nil {
		t.Fatalf("resolveInitTargetPath() error = %v", err)
	}

	want := filepath.Join("/xdg", "fget", "fget.yaml")
	if got != want {
		t.Fatalf("resolveInitTargetPath() = %q, want %q", got, want)
	}
}

func TestResolveInitTargetPath_LocalAndFileConflict(t *testing.T) {
	t.Parallel()

	_, err := resolveInitTargetPath(true, "/tmp/fget.yaml", "/cwd", "/home", "")
	if !errors.Is(err, errConfigInitConflictingTargetFlags) {
		t.Fatalf("resolveInitTargetPath() error = %v, want %v", err, errConfigInitConflictingTargetFlags)
	}
}

func TestResolveInitRoots_DefaultToCwd(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()

	got, err := resolveInitRoots(nil, cwd, "/home")
	if err != nil {
		t.Fatalf("resolveInitRoots() error = %v", err)
	}

	want := []string{cwd}
	if !equalStringSlices(got, want) {
		t.Fatalf("resolveInitRoots() = %v, want %v", got, want)
	}
}

func TestResolveInitRoots_DedupAndSort(t *testing.T) {
	t.Parallel()

	rootA := t.TempDir()
	rootB := t.TempDir()
	home := t.TempDir()
	homeRoot := filepath.Join(home, "projects")
	if err := os.MkdirAll(homeRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(homeRoot) error = %v", err)
	}

	got, err := resolveInitRoots([]string{rootB, "~/projects", rootA, rootB}, "/cwd", home)
	if err != nil {
		t.Fatalf("resolveInitRoots() error = %v", err)
	}

	want := []string{rootA, rootB, homeRoot}
	if !equalStringSlices(got, want) {
		t.Fatalf("resolveInitRoots() = %v, want %v", got, want)
	}
}

func TestResolveInitRoots_InvalidPath(t *testing.T) {
	t.Parallel()

	_, err := resolveInitRoots([]string{"/this/path/does/not/exist"}, "/cwd", "/home")
	if err == nil {
		t.Fatal("resolveInitRoots() expected error")
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestApplyInitConfig_CreateWhenMissing(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "fget.yaml")
	roots := []string{"/tmp/b", "/tmp/a"}

	config, err := applyInitConfig(target, roots, false, false)
	if err != nil {
		t.Fatalf("applyInitConfig() error = %v", err)
	}

	wantRoots := []string{"/tmp/a", "/tmp/b"}
	if !equalStringSlices(config.Roots, wantRoots) {
		t.Fatalf("roots = %v, want %v", config.Roots, wantRoots)
	}
	if config.Version != fconfig.ConfigVersionV2 {
		t.Fatalf("version = %q, want %q", config.Version, fconfig.ConfigVersionV2)
	}

	saved, err := vconfig.LoadConfig[fconfig.Config](target)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !equalStringSlices(saved.Roots, wantRoots) {
		t.Fatalf("saved roots = %v, want %v", saved.Roots, wantRoots)
	}
}

func TestApplyInitConfig_MergeExistingRootsAndPreserveCatalogPath(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "fget.yaml")

	initial := &fconfig.Config{
		Version: fconfig.ConfigVersionV1,
		Roots:   []string{"/a", "/c"},
		Catalog: fconfig.CatalogConfig{Path: "~/custom/catalog.yaml"},
	}
	if err := vconfig.SaveConfig(initial, target); err != nil {
		t.Fatalf("SaveConfig(initial) error = %v", err)
	}

	config, err := applyInitConfig(target, []string{"/b", "/a"}, false, false)
	if err != nil {
		t.Fatalf("applyInitConfig() error = %v", err)
	}

	wantRoots := []string{"/a", "/b", "/c"}
	if !equalStringSlices(config.Roots, wantRoots) {
		t.Fatalf("roots = %v, want %v", config.Roots, wantRoots)
	}
	if config.Catalog.Path != "~/custom/catalog.yaml" {
		t.Fatalf("catalog.path = %q, want %q", config.Catalog.Path, "~/custom/catalog.yaml")
	}
}

func TestApplyInitConfig_ForceOverwrite(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "fget.yaml")
	initial := &fconfig.Config{
		Version: fconfig.ConfigVersionV1,
		Roots:   []string{"/old"},
		Catalog: fconfig.CatalogConfig{Path: "~/custom/catalog.yaml"},
	}
	if err := vconfig.SaveConfig(initial, target); err != nil {
		t.Fatalf("SaveConfig(initial) error = %v", err)
	}

	config, err := applyInitConfig(target, []string{"/new"}, true, false)
	if err != nil {
		t.Fatalf("applyInitConfig() error = %v", err)
	}

	if !equalStringSlices(config.Roots, []string{"/new"}) {
		t.Fatalf("roots = %v, want %v", config.Roots, []string{"/new"})
	}
	if config.Catalog.Path != "" {
		t.Fatalf("catalog.path = %q, want empty", config.Catalog.Path)
	}
}

func TestApplyInitConfig_WithCatalogCreatesSiblingCatalogPath(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "fget.yaml")

	initial := &fconfig.Config{
		Version: fconfig.ConfigVersionV1,
		Roots:   []string{"/old"},
		Catalog: fconfig.CatalogConfig{
			Imports: []string{"/external/fget.yaml"},
		},
	}
	if err := vconfig.SaveConfig(initial, target); err != nil {
		t.Fatalf("SaveConfig(initial) error = %v", err)
	}

	config, err := applyInitConfig(target, []string{"/new"}, false, true)
	if err != nil {
		t.Fatalf("applyInitConfig() error = %v", err)
	}

	if config.Version != fconfig.ConfigVersionV2 {
		t.Fatalf("version = %q, want %q", config.Version, fconfig.ConfigVersionV2)
	}
	if !equalStringSlices(config.Roots, []string{"/new", "/old"}) {
		t.Fatalf("roots = %v, want %v", config.Roots, []string{"/new", "/old"})
	}
	if config.Catalog.Path != fconfig.ResolveScopedCatalogPath() {
		t.Fatalf("catalog.path = %q, want %q", config.Catalog.Path, fconfig.ResolveScopedCatalogPath())
	}
	if !equalStringSlices(config.Catalog.Imports, []string{"/external/fget.yaml"}) {
		t.Fatalf("catalog.imports = %v, want %v", config.Catalog.Imports, []string{"/external/fget.yaml"})
	}
}

func TestRunConfigInit_LocalCreatesConfigFile(t *testing.T) {
	tmp := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir(tmp) error = %v", err)
	}
	expectedRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}

	originalFlags := configInitCmdFlags
	configInitCmdFlags = configInitOptions{
		Local: true,
	}
	defer func() {
		configInitCmdFlags = originalFlags
	}()

	if err := runConfigInit(nil, nil); err != nil {
		t.Fatalf("runConfigInit() error = %v", err)
	}

	target := filepath.Join(tmp, "fget.yaml")
	config, err := vconfig.LoadConfig[fconfig.Config](target)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !equalStringSlices(config.Roots, []string{expectedRoot}) {
		t.Fatalf("roots = %v, want %v", config.Roots, []string{expectedRoot})
	}
}

func TestRunConfigInit_LocalWithCatalogCreatesScopedConfigFile(t *testing.T) {
	tmp := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir(tmp) error = %v", err)
	}

	originalFlags := configInitCmdFlags
	configInitCmdFlags = configInitOptions{
		Local:   true,
		Catalog: true,
	}
	defer func() {
		configInitCmdFlags = originalFlags
	}()

	if err := runConfigInit(nil, nil); err != nil {
		t.Fatalf("runConfigInit() error = %v", err)
	}

	target := filepath.Join(tmp, "fget.yaml")
	config, err := vconfig.LoadConfig[fconfig.Config](target)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.Version != fconfig.ConfigVersionV2 {
		t.Fatalf("version = %q, want %q", config.Version, fconfig.ConfigVersionV2)
	}
	if config.Catalog.Path != fconfig.ResolveScopedCatalogPath() {
		t.Fatalf("catalog.path = %q, want %q", config.Catalog.Path, fconfig.ResolveScopedCatalogPath())
	}
}
