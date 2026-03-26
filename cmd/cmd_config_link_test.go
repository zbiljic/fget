package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zbiljic/fget/pkg/fconfig"
)

func TestResolveLinkConfigForRuntimeContext_ErrorsWithoutLinkConfig(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	cwd := filepath.Join(homeDir, "dev", "wtopic___", "fs___")

	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "fget.yaml"), []byte("version: \"1\"\nroots:\n  - ~/dev/src\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(homeConfigPath) error = %v", err)
	}

	_, err := resolveLinkConfigForRuntimeContext(configRuntimeContext{
		HomeDir:       homeDir,
		Cwd:           cwd,
		XDGConfigHome: "",
	})
	if err == nil {
		t.Fatal("resolveLinkConfigForRuntimeContext() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "no link configuration found") {
		t.Fatalf("resolveLinkConfigForRuntimeContext() error = %q, want no link configuration found", err)
	}
}

func TestResolveLinkConfigForRuntimeContext_LoadsNearestLinkConfig(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	cwd := filepath.Join(homeDir, "dev", "wtopic___", "fs___", "nested")
	projectDir := filepath.Dir(cwd)

	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "fget.yaml"), []byte("version: \"1\"\nroots:\n  - ~/dev/src\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(homeConfigPath) error = %v", err)
	}
	projectConfig := "" +
		"version: \"1\"\n" +
		"link:\n" +
		"  tags:\n" +
		"    - fs___\n" +
		"  root: .\n"
	if err := os.WriteFile(filepath.Join(projectDir, "fget.yaml"), []byte(projectConfig), 0o644); err != nil {
		t.Fatalf("WriteFile(projectConfigPath) error = %v", err)
	}

	config, err := resolveLinkConfigForRuntimeContext(configRuntimeContext{
		HomeDir:       homeDir,
		Cwd:           cwd,
		XDGConfigHome: "",
	})
	if err != nil {
		t.Fatalf("resolveLinkConfigForRuntimeContext() error = %v", err)
	}
	if config.Link == nil {
		t.Fatal("resolved config link = nil, want non-nil")
	}
	if config.Link.Root != projectDir {
		t.Fatalf("resolved link root = %q, want %q", config.Link.Root, projectDir)
	}
}

func TestLoadCatalogSetForEffectiveConfig_MissingCatalog(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	_, err := loadCatalogSetForEffectiveConfig(&fconfig.EffectiveConfig{
		Config: fconfig.Config{
			Catalog: fconfig.CatalogConfig{Path: filepath.Join(homeDir, "missing", "catalog.yaml")},
		},
	}, homeDir)
	if err == nil {
		t.Fatal("loadCatalogSetForEffectiveConfig() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run `fget catalog sync` first") {
		t.Fatalf("loadCatalogSetForEffectiveConfig() error = %q, want sync hint", err)
	}
}
