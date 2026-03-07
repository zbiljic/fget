package cmd

import (
	"errors"
	"reflect"
	"strings"
	"testing"
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
