package fbackup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zbiljic/fget/pkg/gitinspect"
)

func TestRestoreFullIntoEmptyDestination(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mustGitTest(t, root, "init", "-q", repo)
	mustGitTest(t, repo, "config", "user.email", "test@example.com")
	mustGitTest(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repo, "add", "tracked")
	mustGitTest(t, repo, "commit", "-qm", "initial")
	fullState, err := gitinspect.InspectState(context.Background(), repo, gitinspect.CLIRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, "readonly"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "readonly", "child"), []byte("child"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(repo, "readonly"), 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tracked", filepath.Join(repo, "tracked-link")); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "backup")
	if err := Create(context.Background(), CreateOptions{Destination: backup, Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{ID: "full/repo", Path: repo, Classification: ClassificationFull, Git: fullState}}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(repo, "readonly"), 0o755); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(root, "restored")
	if err := Restore(context.Background(), RestoreOptions{Backup: backup, Destination: restored}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(restored, "full", "repo", "tracked"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload" {
		t.Fatalf("restored data = %q", data)
	}
	link, err := os.Lstat(filepath.Join(restored, "full", "repo", "tracked-link"))
	if err != nil {
		t.Fatal(err)
	}
	if link.Mode()&os.ModeSymlink == 0 {
		t.Fatal("full symlink was not preserved")
	}
	mode, err := os.Stat(filepath.Join(restored, "full", "repo", "readonly"))
	if err != nil {
		t.Fatal(err)
	}
	if mode.Mode().Perm() != 0o555 {
		t.Fatalf("readonly mode = %o", mode.Mode().Perm())
	}
	if err := Restore(context.Background(), RestoreOptions{Backup: backup, Destination: restored}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(restored, "full", "repo", "tracked"), []byte("user edit"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := Restore(context.Background(), RestoreOptions{Backup: backup, Destination: restored}); err == nil {
		t.Fatal("modified restored repository was skipped")
	}
	content, err := os.ReadFile(filepath.Join(restored, "full", "repo", "tracked"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "user edit" {
		t.Fatalf("modified content was overwritten: %q", content)
	}
	if err := os.Chmod(filepath.Join(restored, "full", "repo", "readonly"), 0o755); err != nil {
		t.Fatal(err)
	}
}
