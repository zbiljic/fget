package fconfig

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestAddTagsByID_DeduplicatesAndSorts(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{
		Version: CatalogVersionV1,
		Repos: []RepoEntry{
			{
				ID: "github.com/acme/api",
				Locations: []RepoLocation{
					{Path: "/repos/api"},
				},
				Tags: []string{"backend"},
			},
		},
	}

	err := AddTags(catalog, "github.com/acme/api", []string{"zeta", "backend", "alpha"})
	if err != nil {
		t.Fatalf("AddTags() error = %v", err)
	}

	want := []string{"alpha", "backend", "zeta"}
	if !reflect.DeepEqual(catalog.Repos[0].Tags, want) {
		t.Fatalf("catalog tags = %v, want %v", catalog.Repos[0].Tags, want)
	}
}

func TestRemoveTagsByPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join("/repos", "api")
	catalog := &Catalog{
		Version: CatalogVersionV1,
		Repos: []RepoEntry{
			{
				ID: "github.com/acme/api",
				Locations: []RepoLocation{
					{Path: path},
				},
				Tags: []string{"alpha", "backend", "zeta"},
			},
		},
	}

	err := RemoveTags(catalog, path, []string{"backend", "missing"})
	if err != nil {
		t.Fatalf("RemoveTags() error = %v", err)
	}

	want := []string{"alpha", "zeta"}
	if !reflect.DeepEqual(catalog.Repos[0].Tags, want) {
		t.Fatalf("catalog tags = %v, want %v", catalog.Repos[0].Tags, want)
	}
}

func TestResolveRepoIndex_UnknownSelector(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{
		Version: CatalogVersionV1,
		Repos: []RepoEntry{
			{
				ID: "github.com/acme/api",
				Locations: []RepoLocation{
					{Path: "/repos/api"},
				},
			},
		},
	}

	_, err := ResolveRepoIndex(catalog, "github.com/acme/unknown")
	if err == nil {
		t.Fatal("ResolveRepoIndex() expected error")
	}
}
