package cmd

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-git/go-git/v5"
	gitcfg "github.com/go-git/go-git/v5/config"
)

func TestInspectSyncRepoMetadata(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()

	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("PlainInit() error = %v", err)
	}

	_, err = repo.CreateRemote(&gitcfg.RemoteConfig{
		Name: git.DefaultRemoteName,
		URLs: []string{"https://github.com/acme/api.git"},
	})
	if err != nil {
		t.Fatalf("CreateRemote() error = %v", err)
	}

	got, err := inspectSyncRepoMetadata(repoDir)
	if err != nil {
		t.Fatalf("inspectSyncRepoMetadata() error = %v", err)
	}

	want := syncRepoMetadata{
		ID:        filepath.Join("github.com", "acme", "api"),
		RemoteURL: "https://github.com/acme/api.git",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inspectSyncRepoMetadata() = %+v, want %+v", got, want)
	}
}
