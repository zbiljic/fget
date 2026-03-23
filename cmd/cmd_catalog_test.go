package cmd

import (
	"reflect"
	"testing"

	"github.com/zbiljic/fget/pkg/fconfig"
)

func TestSelectCatalogRepos_ReturnsAllWhenSelectorsOmitted(t *testing.T) {
	t.Parallel()

	catalog := &fconfig.Catalog{
		Repos: []fconfig.RepoEntry{
			{ID: "github.com/acme/api"},
			{ID: "github.com/acme/web"},
		},
	}

	repos, err := selectCatalogRepos(catalog, nil)
	if err != nil {
		t.Fatalf("selectCatalogRepos() error = %v", err)
	}

	if !reflect.DeepEqual(repos, catalog.Repos) {
		t.Fatalf("selectCatalogRepos() = %v, want %v", repos, catalog.Repos)
	}
}

func TestSelectCatalogRepos_NormalizesSelectorsAndDeduplicates(t *testing.T) {
	t.Parallel()

	catalog := &fconfig.Catalog{
		Repos: []fconfig.RepoEntry{
			{
				ID: "example.com/acme/api",
				Locations: []fconfig.RepoLocation{
					{Path: "/workspace/api"},
				},
			},
			{
				ID: "github.com/acme/web",
				Locations: []fconfig.RepoLocation{
					{Path: "/workspace/web"},
				},
			},
		},
	}

	repos, err := selectCatalogRepos(catalog, []string{
		"/workspace/api",
		"https://github.com/acme/web",
		"example.com/acme/api",
	})
	if err != nil {
		t.Fatalf("selectCatalogRepos() error = %v", err)
	}

	gotIDs := make([]string, 0, len(repos))
	for _, repo := range repos {
		gotIDs = append(gotIDs, repo.ID)
	}

	wantIDs := []string{"example.com/acme/api", "github.com/acme/web"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("selectCatalogRepos() ids = %v, want %v", gotIDs, wantIDs)
	}
}
