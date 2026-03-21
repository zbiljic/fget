package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zbiljic/fget/pkg/fconfig"
	"github.com/zbiljic/fget/pkg/vconfig"
)

func TestBuildLinkInitConfig_UsesDefaults(t *testing.T) {
	t.Parallel()

	link, err := buildLinkInitConfig(nil, []string{" alpha ", "beta", "alpha"}, configLinkInitOptions{})
	if err != nil {
		t.Fatalf("buildLinkInitConfig() error = %v", err)
	}

	wantTags := []string{"alpha", "beta"}
	if !equalStringSlices(link.Tags, wantTags) {
		t.Fatalf("link.tags = %v, want %v", link.Tags, wantTags)
	}
	if link.Match != fconfig.LinkMatchAny {
		t.Fatalf("link.match = %q, want %q", link.Match, fconfig.LinkMatchAny)
	}
	if link.Layout != fconfig.LinkLayoutRepoID {
		t.Fatalf("link.layout = %q, want %q", link.Layout, fconfig.LinkLayoutRepoID)
	}
	if link.Root != "." {
		t.Fatalf("link.root = %q, want %q", link.Root, ".")
	}
	if link.SourceRoot != "" {
		t.Fatalf("link.source_root = %q, want empty", link.SourceRoot)
	}
}

func TestBuildLinkInitConfig_PreservesExistingSettings(t *testing.T) {
	t.Parallel()

	link, err := buildLinkInitConfig(&fconfig.LinkConfig{
		Tags:       []string{"old"},
		Match:      fconfig.LinkMatchAll,
		Layout:     fconfig.LinkLayoutRepoID,
		Root:       "links",
		SourceRoot: "~/dev/src",
	}, []string{"fs___"}, configLinkInitOptions{})
	if err != nil {
		t.Fatalf("buildLinkInitConfig() error = %v", err)
	}

	if !equalStringSlices(link.Tags, []string{"fs___"}) {
		t.Fatalf("link.tags = %v, want %v", link.Tags, []string{"fs___"})
	}
	if link.Match != fconfig.LinkMatchAll {
		t.Fatalf("link.match = %q, want %q", link.Match, fconfig.LinkMatchAll)
	}
	if link.Root != "links" {
		t.Fatalf("link.root = %q, want %q", link.Root, "links")
	}
	if link.SourceRoot != "~/dev/src" {
		t.Fatalf("link.source_root = %q, want %q", link.SourceRoot, "~/dev/src")
	}
}

func TestBuildLinkInitConfig_InvalidMatch(t *testing.T) {
	t.Parallel()

	_, err := buildLinkInitConfig(nil, []string{"fs___"}, configLinkInitOptions{Match: "nope"})
	if err == nil {
		t.Fatal("buildLinkInitConfig() error = nil, want error")
	}
}

func TestApplyLinkInitConfig_PreservesExistingConfig(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "fget.yaml")

	initial := &fconfig.Config{
		Version: fconfig.ConfigVersionV1,
		Roots:   []string{"/workspace/project-root"},
		Catalog: fconfig.CatalogConfig{Path: ""},
		Link: &fconfig.LinkConfig{
			Tags: []string{"old"},
			Root: "old-links",
		},
	}
	if err := vconfig.SaveConfig(initial, target); err != nil {
		t.Fatalf("SaveConfig(initial) error = %v", err)
	}

	config, err := applyLinkInitConfig(target, fconfig.LinkConfig{
		Tags:   []string{"fs___"},
		Match:  fconfig.LinkMatchAny,
		Layout: fconfig.LinkLayoutRepoID,
		Root:   ".",
	})
	if err != nil {
		t.Fatalf("applyLinkInitConfig() error = %v", err)
	}

	if !equalStringSlices(config.Roots, initial.Roots) {
		t.Fatalf("roots = %v, want %v", config.Roots, initial.Roots)
	}
	if config.Catalog.Path != "" {
		t.Fatalf("catalog.path = %q, want empty", config.Catalog.Path)
	}
	if config.Link == nil {
		t.Fatal("config.link = nil, want non-nil")
	}
	if !equalStringSlices(config.Link.Tags, []string{"fs___"}) {
		t.Fatalf("link.tags = %v, want %v", config.Link.Tags, []string{"fs___"})
	}
	if config.Link.Root != "." {
		t.Fatalf("link.root = %q, want %q", config.Link.Root, ".")
	}
}

func TestRunConfigLinkInit_CreatesLocalConfigFile(t *testing.T) {
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

	originalFlags := configLinkInitCmdFlags
	configLinkInitCmdFlags = configLinkInitOptions{
		Root: ".",
	}
	defer func() {
		configLinkInitCmdFlags = originalFlags
	}()

	if err := runConfigLinkInit(nil, []string{"alpha", "beta"}); err != nil {
		t.Fatalf("runConfigLinkInit() error = %v", err)
	}

	config, err := vconfig.LoadConfig[fconfig.Config](filepath.Join(tmp, "fget.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.Link == nil {
		t.Fatal("config.link = nil, want non-nil")
	}
	if !equalStringSlices(config.Link.Tags, []string{"alpha", "beta"}) {
		t.Fatalf("link.tags = %v, want %v", config.Link.Tags, []string{"alpha", "beta"})
	}
	if config.Link.Match != fconfig.LinkMatchAny {
		t.Fatalf("link.match = %q, want %q", config.Link.Match, fconfig.LinkMatchAny)
	}
}
