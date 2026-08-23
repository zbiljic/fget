package fbackup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zbiljic/fget/pkg/gitinspect"
)

type restoreRecordingRunner struct{ calls int }

func (runner *restoreRecordingRunner) Run(_ context.Context, _ string, _ ...string) (gitinspect.Result, error) {
	runner.calls++
	return gitinspect.Result{}, nil
}

func TestRestoreRejectsCorruptBackupBeforePublishing(t *testing.T) {
	repo := newRecloneableRepository(t, "full")
	repo.Classification = ClassificationFull
	backup := filepath.Join(t.TempDir(), "backup")
	if err := Create(context.Background(), CreateOptions{Destination: backup, Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{repo}}}); err != nil {
		t.Fatal(err)
	}
	metadata, err := readBackupMetadata(filepath.Join(backup, "backup.json"))
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(backup, filepath.FromSlash(metadata.Artifacts[0].Path))
	if err := os.WriteFile(artifact, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "destination")
	if err := Restore(context.Background(), RestoreOptions{Backup: backup, Destination: destination}); err == nil {
		t.Fatal("corrupt backup restored")
	}
	if _, err := os.Stat(filepath.Join(destination, "full")); !os.IsNotExist(err) {
		t.Fatalf("destination gained repository path: %v", err)
	}
}

func TestRestorePatchConflictDoesNotPublishRepository(t *testing.T) {
	repository := newRecloneableRepository(t, "delta")
	if err := os.WriteFile(filepath.Join(repository.Path, "tracked"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	repository.Classification = ClassificationDelta
	backup := filepath.Join(t.TempDir(), "backup")
	if err := Create(context.Background(), CreateOptions{Destination: backup, Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{repository}}}); err != nil {
		t.Fatal(err)
	}
	metadata, err := readBackupMetadata(filepath.Join(backup, "backup.json"))
	if err != nil {
		t.Fatal(err)
	}
	for index := range metadata.Artifacts {
		if metadata.Artifacts[index].Kind == "patch" {
			path := filepath.Join(backup, filepath.FromSlash(metadata.Artifacts[index].Path))
			bad := []byte("this is not a patch\n")
			if err := os.WriteFile(path, bad, 0o600); err != nil {
				t.Fatal(err)
			}
			metadata.Artifacts[index].Size = int64(len(bad))
			_, metadata.Artifacts[index].SHA256, err = hashFile(path)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writeJSONAtomic(filepath.Join(backup, "backup.json"), metadata, 0o644, ".fget-backup-metadata-*.tmp"); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "destination")
	if err := Restore(context.Background(), RestoreOptions{Backup: backup, Destination: destination}); err == nil {
		t.Fatal("patch conflict restored")
	}
	if _, err := os.Stat(filepath.Join(destination, "delta")); !os.IsNotExist(err) {
		t.Fatalf("partial repository published: %v", err)
	}
}

func TestRestoreDryRunDoesNotWriteDestination(t *testing.T) {
	repository := newRecloneableRepository(t, "full")
	repository.Classification = ClassificationFull
	backup := filepath.Join(t.TempDir(), "backup")
	if err := Create(context.Background(), CreateOptions{Destination: backup, Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{repository}}}); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "destination")
	report, err := RestoreWithReport(context.Background(), RestoreOptions{Backup: backup, Destination: destination, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Planned) != 1 {
		t.Fatalf("planned = %#v", report.Planned)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, restoreStateFile)); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote restore state %q: %v", restoreStateFile, err)
	}
}

func TestRestoreDryRunRecloneableMakesNoCloneCall(t *testing.T) {
	repository := newRecloneableRepository(t, "recloneable")
	backup := filepath.Join(t.TempDir(), "backup")
	if err := Create(context.Background(), CreateOptions{Destination: backup, Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{repository}}}); err != nil {
		t.Fatal(err)
	}
	runner := &restoreRecordingRunner{}
	report, err := RestoreWithReport(context.Background(), RestoreOptions{Backup: backup, Destination: filepath.Join(t.TempDir(), "destination"), DryRun: true, GitRunner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Planned) != 1 || runner.calls != 0 {
		t.Fatalf("dry-run report=%+v calls=%d", report, runner.calls)
	}
}

func TestRestoreRejectsCaseFoldAndAncestorCollisions(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "destination")
	if err := validateRestoreDestination(destination, []RepositoryEntry{{ID: "Org/Repo"}, {ID: "org/repo"}}, true); err == nil {
		t.Fatal("case-fold collision accepted")
	}
	if err := validateRestoreDestination(destination, []RepositoryEntry{{ID: "a"}, {ID: "a/b"}}, true); err == nil {
		t.Fatal("ancestor collision accepted")
	}
	if err := validateRestoreDestination(destination, []RepositoryEntry{{ID: "A"}, {ID: "a/b"}}, true); err == nil {
		t.Fatal("case-fold ancestor collision accepted")
	}
}

func TestRestoreRejectsSymlinkAncestorAndPreexistingData(t *testing.T) {
	root := t.TempDir()
	realRoot := filepath.Join(root, "real")
	if err := os.Mkdir(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(realRoot, alias); err != nil {
		t.Fatal(err)
	}
	if err := validateRestoreDestination(filepath.Join(alias, "dest"), []RepositoryEntry{{ID: "repo"}}, true); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	destination := filepath.Join(root, "existing")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "user-data"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateRestoreDestination(destination, []RepositoryEntry{{ID: "repo"}}, false); err == nil {
		t.Fatal("preexisting data accepted")
	}
}

func TestOpenRestoreDestinationReturnsSpecificResumeError(t *testing.T) {
	destination := t.TempDir()
	backup := "/backup"
	metadata := BackupMetadata{SourceAuditHash: "audit"}
	state := restoreState{SchemaVersion: BackupSchemaVersion, Backup: backup, SourceAuditHash: metadata.SourceAuditHash}
	if err := writeJSONAtomic(filepath.Join(destination, restoreStateFile), state, 0o600, ".fget-restore-state-*.tmp"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(destination, "unrelated-empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := openRestoreDestination(destination, backup, metadata, nil)
	if err == nil || err.Error() != "restore destination contains unrelated data" {
		t.Fatalf("openRestoreDestination() error = %v", err)
	}
}

func TestValidateResumeDestinationRejectsUnrelatedEmptyDirectory(t *testing.T) {
	destination := t.TempDir()
	completed := filepath.Join(destination, "org", "repo")
	if err := os.MkdirAll(completed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(completed, "tracked"), []byte("restored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, restoreStateFile), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(destination, "unrelated-empty")
	if err := os.Mkdir(unrelated, 0o755); err != nil {
		t.Fatal(err)
	}

	state := restoreState{Completed: []string{"org/repo"}}
	if err := validateResumeDestination(destination, state); err == nil {
		t.Fatal("resume accepted an unrelated empty directory")
	}
	if err := os.Remove(unrelated); err != nil {
		t.Fatal(err)
	}
	if err := validateResumeDestination(destination, state); err != nil {
		t.Fatalf("valid completed repository tree rejected: %v", err)
	}

	symlinkDestination := t.TempDir()
	if err := os.WriteFile(filepath.Join(symlinkDestination, restoreStateFile), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(symlinkDestination, "org")); err != nil {
		t.Fatal(err)
	}
	if err := validateResumeDestination(symlinkDestination, state); err == nil {
		t.Fatal("resume accepted a symlink ancestor of a completed repository")
	}
}

func TestRestoreStateIsBoundToAuditHash(t *testing.T) {
	destination := t.TempDir()
	data, err := json.Marshal(restoreState{SchemaVersion: BackupSchemaVersion, Backup: "/backup", SourceAuditHash: "old"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, restoreStateFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRestoreState(destination, filepath.Join(t.TempDir(), "backup"), BackupMetadata{SourceAuditHash: "new"}); err == nil {
		t.Fatal("invalid state accepted")
	}
}

func TestRestoreCancellationResumesCompletedRepository(t *testing.T) {
	root := t.TempDir()
	entries := make([]RepositoryEntry, 0, 2)
	for _, id := range []string{"a", "b"} {
		repo := filepath.Join(root, id)
		mustGitTest(t, root, "init", "-q", repo)
		mustGitTest(t, repo, "config", "user.email", "test@example.com")
		mustGitTest(t, repo, "config", "user.name", "Test")
		if err := os.WriteFile(filepath.Join(repo, "file"), []byte(id), 0o640); err != nil {
			t.Fatal(err)
		}
		mustGitTest(t, repo, "add", "file")
		mustGitTest(t, repo, "commit", "-qm", "initial")
		state, err := gitinspect.InspectState(context.Background(), repo, gitinspect.CLIRunner{})
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, RepositoryEntry{ID: id, Path: repo, Classification: ClassificationFull, Git: state})
	}
	backup := filepath.Join(root, "backup")
	if err := Create(context.Background(), CreateOptions{Destination: backup, Manifest: Manifest{Version: ManifestVersion, Repositories: entries}}); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "destination")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := RestoreWithReport(ctx, RestoreOptions{Backup: backup, Destination: destination, Progress: func(id, status string) {
		if id == "a" && status == "complete" {
			cancel()
		}
	}})
	if err == nil {
		t.Fatal("canceled restore succeeded")
	}
	if _, err := os.Stat(filepath.Join(destination, restoreStateFile)); err != nil {
		t.Fatalf("resume state missing: %v", err)
	}
	report, err := RestoreWithReport(context.Background(), RestoreOptions{Backup: backup, Destination: destination})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Skipped) != 1 || report.Skipped[0] != "a" || len(report.Restored) != 1 || report.Restored[0] != "b" {
		t.Fatalf("resume report = %+v", report)
	}
}
