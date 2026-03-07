package cmd

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zbiljic/fget/pkg/fconfig"
)

func TestParseConfigTagModifyArgs(t *testing.T) {
	t.Parallel()

	repo, tags, err := parseConfigTagModifyArgs(
		[]string{"github.com/acme/api", "alpha", "beta"},
		"/workspace",
		func(string) (string, error) {
			t.Fatal("gitRoot should not be called when repo selector is explicitly provided")
			return "", nil
		},
	)
	if err != nil {
		t.Fatalf("parseConfigTagModifyArgs() error = %v", err)
	}

	if repo != "github.com/acme/api" {
		t.Fatalf("parseConfigTagModifyArgs() repo = %q, want %q", repo, "github.com/acme/api")
	}

	wantTags := []string{"alpha", "beta"}
	if !reflect.DeepEqual(tags, wantTags) {
		t.Fatalf("parseConfigTagModifyArgs() tags = %v, want %v", tags, wantTags)
	}
}

func TestParseConfigTagModifyArgs_InfersRepoSelectorFromGitRoot(t *testing.T) {
	t.Parallel()

	cwd := "/workspace/service/subdir"
	repo, tags, err := parseConfigTagModifyArgs([]string{"alpha"}, cwd, func(path string) (string, error) {
		if path != cwd {
			t.Fatalf("gitRoot() path = %q, want %q", path, cwd)
		}
		return "/workspace/service", nil
	})
	if err != nil {
		t.Fatalf("parseConfigTagModifyArgs() error = %v", err)
	}

	if repo != "/workspace/service" {
		t.Fatalf("parseConfigTagModifyArgs() repo = %q, want %q", repo, "/workspace/service")
	}

	wantTags := []string{"alpha"}
	if !reflect.DeepEqual(tags, wantTags) {
		t.Fatalf("parseConfigTagModifyArgs() tags = %v, want %v", tags, wantTags)
	}
}

func TestParseConfigTagModifyArgs_RequiresRepoOrGitContext(t *testing.T) {
	t.Parallel()

	_, _, err := parseConfigTagModifyArgs([]string{"alpha"}, "/workspace", func(string) (string, error) {
		return "", errors.New("not a git repository")
	})
	if err == nil {
		t.Fatal("parseConfigTagModifyArgs() expected error when repository selector is omitted outside git repository")
	}

	if !strings.Contains(err.Error(), "requires a repository selector") {
		t.Fatalf("parseConfigTagModifyArgs() error = %q, want mention of repository selector requirement", err)
	}
}

func TestParseConfigTagModifyArgs_RequiresAtLeastOneTag(t *testing.T) {
	t.Parallel()

	_, _, err := parseConfigTagModifyArgs([]string{}, "/workspace", func(string) (string, error) {
		t.Fatal("gitRoot should not be called when there are no args")
		return "", nil
	})
	if err == nil {
		t.Fatal("parseConfigTagModifyArgs() expected error for missing tags")
	}
}

func TestResolveConfigTagModifyRequest_ExplicitRepoSelector(t *testing.T) {
	t.Parallel()

	catalog := &fconfig.Catalog{
		Repos: []fconfig.RepoEntry{
			{
				ID: "github.com/acme/api",
				Locations: []fconfig.RepoLocation{
					{Path: "/workspace/api"},
				},
			},
		},
	}

	req, err := resolveConfigTagModifyRequest(
		context.Background(),
		[]string{"github.com/acme/api", "alpha"},
		"/workspace",
		catalog,
		func(string) (string, error) {
			t.Fatal("gitRoot should not be called when explicit repo selector is provided")
			return "", nil
		},
		func(context.Context, ...string) ([]string, error) {
			t.Fatal("findRepos should not be called when explicit repo selector is provided")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("resolveConfigTagModifyRequest() error = %v", err)
	}

	if !reflect.DeepEqual(req.RepoSelectors, []string{"github.com/acme/api"}) {
		t.Fatalf("resolveConfigTagModifyRequest() selectors = %v, want %v", req.RepoSelectors, []string{"github.com/acme/api"})
	}
	if !reflect.DeepEqual(req.Tags, []string{"alpha"}) {
		t.Fatalf("resolveConfigTagModifyRequest() tags = %v, want %v", req.Tags, []string{"alpha"})
	}
	if req.RequiresConfirmation {
		t.Fatal("resolveConfigTagModifyRequest() RequiresConfirmation = true, want false")
	}
}

func TestResolveConfigTagModifyRequest_BulkFromSearchWhenOutsideRepo(t *testing.T) {
	t.Parallel()

	catalog := &fconfig.Catalog{
		Repos: []fconfig.RepoEntry{
			{
				ID: "github.com/acme/api",
				Locations: []fconfig.RepoLocation{
					{Path: "/workspace/repos/api"},
				},
			},
			{
				ID: "github.com/acme/web",
				Locations: []fconfig.RepoLocation{
					{Path: "/workspace/repos/web"},
				},
			},
		},
	}

	req, err := resolveConfigTagModifyRequest(
		context.Background(),
		[]string{"team-a", "critical"},
		"/workspace",
		catalog,
		func(string) (string, error) {
			return "", errors.New("not in git repo")
		},
		func(_ context.Context, roots ...string) ([]string, error) {
			if !reflect.DeepEqual(roots, []string{"/workspace"}) {
				t.Fatalf("findRepos() roots = %v, want %v", roots, []string{"/workspace"})
			}
			return []string{"/workspace/repos/web", "/workspace/repos/api"}, nil
		},
	)
	if err != nil {
		t.Fatalf("resolveConfigTagModifyRequest() error = %v", err)
	}

	wantSelectors := []string{"github.com/acme/api", "github.com/acme/web"}
	if !reflect.DeepEqual(req.RepoSelectors, wantSelectors) {
		t.Fatalf("resolveConfigTagModifyRequest() selectors = %v, want %v", req.RepoSelectors, wantSelectors)
	}
	if !reflect.DeepEqual(req.Tags, []string{"team-a", "critical"}) {
		t.Fatalf("resolveConfigTagModifyRequest() tags = %v, want %v", req.Tags, []string{"team-a", "critical"})
	}
	if !req.RequiresConfirmation {
		t.Fatal("resolveConfigTagModifyRequest() RequiresConfirmation = false, want true")
	}
}

func TestResolveConfigTagModifyRequest_BulkFromSearchNoCatalogMatches(t *testing.T) {
	t.Parallel()

	catalog := &fconfig.Catalog{
		Repos: []fconfig.RepoEntry{
			{
				ID: "github.com/acme/api",
				Locations: []fconfig.RepoLocation{
					{Path: "/workspace/repos/api"},
				},
			},
		},
	}

	_, err := resolveConfigTagModifyRequest(
		context.Background(),
		[]string{"team-a"},
		"/workspace",
		catalog,
		func(string) (string, error) {
			return "", errors.New("not in git repo")
		},
		func(context.Context, ...string) ([]string, error) {
			return []string{"/workspace/repos/other"}, nil
		},
	)
	if err == nil {
		t.Fatal("resolveConfigTagModifyRequest() expected error")
	}
	if !strings.Contains(err.Error(), "no catalog repositories found") {
		t.Fatalf("resolveConfigTagModifyRequest() error = %q, want no catalog repositories found", err)
	}
}

func TestConfigTagModifyConfirmText_ListsRepositories(t *testing.T) {
	t.Parallel()

	req := configTagModifyRequest{
		RepoSelectors: []string{
			"github.com/acme/api",
			"github.com/acme/web",
		},
		Tags: []string{"team-a", "critical"},
	}

	got := configTagModifyConfirmText("add", req)

	if !strings.Contains(got, "add tags [team-a,critical] on 2 discovered repositories:") {
		t.Fatalf("configTagModifyConfirmText() = %q, want summary header", got)
	}
	if !strings.Contains(got, " - github.com/acme/api") {
		t.Fatalf("configTagModifyConfirmText() = %q, want api repo in list", got)
	}
	if !strings.Contains(got, " - github.com/acme/web") {
		t.Fatalf("configTagModifyConfirmText() = %q, want web repo in list", got)
	}
	if !strings.Contains(got, "continue?") {
		t.Fatalf("configTagModifyConfirmText() = %q, want continue text", got)
	}
}
