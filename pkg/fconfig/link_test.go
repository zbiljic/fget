package fconfig

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveLinkTargets_UsesSourceRootAndRepoIDLayout(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{
		Repos: []RepoEntry{
			{
				ID:   "github.com/cli/cli",
				Tags: []string{"fs___"},
				Locations: []RepoLocation{
					{Path: "/tmp/elsewhere/github.com/cli/cli"},
					{Path: "/Users/me/dev/src/github.com/cli/cli"},
				},
			},
		},
	}

	spec := LinkConfig{
		Tags:       []string{"fs___"},
		Match:      "any",
		Layout:     "repo-id",
		Root:       "/Users/me/dev/wtopic___/fs___",
		SourceRoot: "/Users/me/dev/src",
	}

	targets, problems := ResolveLinkTargets(catalog, spec)
	if len(problems) != 0 {
		t.Fatalf("ResolveLinkTargets() problems = %v, want none", problems)
	}

	want := []LinkTarget{
		{
			RepoID:     "github.com/cli/cli",
			SourcePath: "/Users/me/dev/src/github.com/cli/cli",
			TargetPath: filepath.Join("/Users/me/dev/wtopic___/fs___", "github.com", "cli", "cli"),
		},
	}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("ResolveLinkTargets() = %v, want %v", targets, want)
	}
}

func TestResolveLinkTargets_MatchAll(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{
		Repos: []RepoEntry{
			{
				ID:        "github.com/acme/api",
				Tags:      []string{"backend", "fs___"},
				Locations: []RepoLocation{{Path: "/src/github.com/acme/api"}},
			},
			{
				ID:        "github.com/acme/web",
				Tags:      []string{"fs___"},
				Locations: []RepoLocation{{Path: "/src/github.com/acme/web"}},
			},
		},
	}

	spec := LinkConfig{
		Tags:   []string{"backend", "fs___"},
		Match:  "all",
		Layout: "repo-id",
		Root:   "/links",
	}

	targets, problems := ResolveLinkTargets(catalog, spec)
	if len(problems) != 0 {
		t.Fatalf("ResolveLinkTargets() problems = %v, want none", problems)
	}
	if len(targets) != 1 {
		t.Fatalf("ResolveLinkTargets() target count = %d, want 1", len(targets))
	}
	if targets[0].RepoID != "github.com/acme/api" {
		t.Fatalf("ResolveLinkTargets() repo = %q, want %q", targets[0].RepoID, "github.com/acme/api")
	}
}

func TestResolveLinkTargets_AmbiguousWithoutSourceRoot(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{
		Repos: []RepoEntry{
			{
				ID:   "github.com/acme/api",
				Tags: []string{"fs___"},
				Locations: []RepoLocation{
					{Path: "/src-a/github.com/acme/api"},
					{Path: "/src-b/github.com/acme/api"},
				},
			},
		},
	}

	spec := LinkConfig{
		Tags:   []string{"fs___"},
		Match:  "any",
		Layout: "repo-id",
		Root:   "/links",
	}

	targets, problems := ResolveLinkTargets(catalog, spec)
	if len(targets) != 0 {
		t.Fatalf("ResolveLinkTargets() targets = %v, want none", targets)
	}
	if len(problems) != 1 {
		t.Fatalf("ResolveLinkTargets() problems = %v, want 1", problems)
	}
	if problems[0].RepoID != "github.com/acme/api" {
		t.Fatalf("problem repo = %q, want %q", problems[0].RepoID, "github.com/acme/api")
	}
	if !strings.Contains(problems[0].Err.Error(), "multiple catalog locations") {
		t.Fatalf("problem error = %q, want multiple catalog locations", problems[0].Err)
	}
}

func TestResolveLinkTargets_AmbiguousWithMultipleSourceRootMatches(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{
		Repos: []RepoEntry{
			{
				ID:   "github.com/acme/api",
				Tags: []string{"fs___"},
				Locations: []RepoLocation{
					{Path: "/src/github.com/acme/api"},
					{Path: "/src/mirror/github.com/acme/api"},
				},
			},
		},
	}

	spec := LinkConfig{
		Tags:       []string{"fs___"},
		Match:      "any",
		Layout:     "repo-id",
		Root:       "/links",
		SourceRoot: "/src",
	}

	targets, problems := ResolveLinkTargets(catalog, spec)
	if len(targets) != 0 {
		t.Fatalf("ResolveLinkTargets() targets = %v, want none", targets)
	}
	if len(problems) != 1 {
		t.Fatalf("ResolveLinkTargets() problems = %v, want 1", problems)
	}
	if !strings.Contains(problems[0].Err.Error(), "multiple locations under source_root") {
		t.Fatalf("problem error = %q, want multiple locations under source_root", problems[0].Err)
	}
}

func TestResolveLinkTargets_InvalidLayout(t *testing.T) {
	t.Parallel()

	_, problems := ResolveLinkTargets(&Catalog{}, LinkConfig{
		Tags:   []string{"fs___"},
		Layout: "flat",
		Root:   "/links",
	})
	if len(problems) != 1 {
		t.Fatalf("ResolveLinkTargets() problems = %v, want 1", problems)
	}
	if !errors.Is(problems[0].Err, errInvalidLinkLayout) {
		t.Fatalf("problem error = %v, want %v", problems[0].Err, errInvalidLinkLayout)
	}
}

func TestResolveLinkTargets_EmptyTags(t *testing.T) {
	t.Parallel()

	_, problems := ResolveLinkTargets(&Catalog{}, LinkConfig{
		Layout: "repo-id",
		Root:   "/links",
	})
	if len(problems) != 1 {
		t.Fatalf("ResolveLinkTargets() problems = %v, want 1", problems)
	}
	if !errors.Is(problems[0].Err, errEmptyLinkTags) {
		t.Fatalf("problem error = %v, want %v", problems[0].Err, errEmptyLinkTags)
	}
}
