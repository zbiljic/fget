package cmd

import (
	"reflect"
	"testing"
)

func TestParseConfigTagModifyArgs(t *testing.T) {
	t.Parallel()

	repo, tags, err := parseConfigTagModifyArgs([]string{"github.com/acme/api", "alpha", "beta"})
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

func TestParseConfigTagModifyArgs_RequiresRepoAndTag(t *testing.T) {
	t.Parallel()

	_, _, err := parseConfigTagModifyArgs([]string{"github.com/acme/api"})
	if err == nil {
		t.Fatal("parseConfigTagModifyArgs() expected error for missing tags")
	}
}
