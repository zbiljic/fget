package fbackup

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zbiljic/fget/pkg/gitinspect"
)

func mustGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, output)
	}
}

func newRecloneableRepository(t *testing.T, id string) RepositoryEntry {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	repository := filepath.Join(root, "repository")
	mustGitTest(t, root, "init", "--bare", "-q", remote)
	mustGitTest(t, root, "init", "-q", repository)
	mustGitTest(t, repository, "config", "user.email", "test@example.com")
	mustGitTest(t, repository, "config", "user.name", "Test")
	mustGitTest(t, repository, "commit", "--allow-empty", "-qm", "initial")
	mustGitTest(t, repository, "remote", "add", "origin", remote)
	mustGitTest(t, repository, "push", "-qu", "origin", "HEAD")
	state, err := gitinspect.InspectState(context.Background(), repository, gitinspect.CLIRunner{})
	if err != nil {
		t.Fatal(err)
	}
	return RepositoryEntry{
		ID:             id,
		Path:           repository,
		RemoteURL:      remote,
		Classification: ClassificationRecloneable,
		Git:            state,
		RemoteState:    RemoteStateReachable,
	}
}
