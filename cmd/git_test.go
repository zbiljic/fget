package cmd

import (
	"context"
	"errors"
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

func TestGitMetadataHelpersRespectCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		run  func(context.Context) error
	}{
		{
			name: "status",
			run: func(ctx context.Context) error {
				_, err := gitRepoIsCleanContext(ctx, t.TempDir())
				return err
			},
		},
		{
			name: "last commit date",
			run: func(ctx context.Context) error {
				_, err := gitLastCommitDateContext(ctx, t.TempDir())
				return err
			},
		},
		{
			name: "commit count",
			run: func(ctx context.Context) error {
				_, err := gitRepoCommitCountContext(ctx, t.TempDir())
				return err
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.run(ctx)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s error = %v, want %v", tt.name, err, context.Canceled)
			}
		})
	}
}
