package fbackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateFullIncludesGitAndLFSData(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mustGitTest(t, root, "init", "-q", repo)
	mustGitTest(t, repo, "config", "user.email", "test@example.com")
	mustGitTest(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("tracked"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeTracked, err := os.Stat(filepath.Join(repo, "tracked"))
	if err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repo, "add", "tracked")
	mustGitTest(t, repo, "commit", "-qm", "initial")
	lfsObject := filepath.Join(repo, ".git", "lfs", "objects", "aa")
	if err := os.MkdirAll(lfsObject, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lfsObject, "object"), []byte("lfs-shaped"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{ID: "full/repo", Path: repo, Classification: ClassificationFull}}}
	destination := filepath.Join(root, "backup")
	if err := Create(context.Background(), CreateOptions{Destination: destination, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	afterTracked, err := os.Stat(filepath.Join(repo, "tracked"))
	if err != nil {
		t.Fatal(err)
	}
	if beforeTracked.Size() != afterTracked.Size() || !beforeTracked.ModTime().Equal(afterTracked.ModTime()) {
		t.Fatal("full backup changed source content or mtime")
	}
	if err := Verify(context.Background(), destination, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "repos", repositoryDirName("full/repo"), "full.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("full archive is empty")
	}
	fullPath := filepath.Join(destination, "repos", repositoryDirName("full/repo"), "full.tar.gz")
	corruptFull := append([]byte(nil), data...)
	corruptFull[0] ^= 0xff
	if err := os.WriteFile(fullPath, corruptFull, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), destination, false); err == nil || !strings.Contains(err.Error(), "full/repo") {
		t.Fatalf("Verify() for corrupt full archive error = %v", err)
	}
	if err := os.WriteFile(fullPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	archiveReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	archiveTar := tar.NewReader(archiveReader)
	wantGit, wantLFS := false, false
	for {
		h, nextErr := archiveTar.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		if h.Name == ".git" || strings.HasPrefix(h.Name, ".git/") {
			wantGit = true
		}
		if h.Name == ".git/lfs/objects/aa/object" {
			wantLFS = true
		}
		if _, err := io.Copy(io.Discard, archiveTar); err != nil {
			t.Fatal(err)
		}
	}
	_ = archiveReader.Close()
	if !wantGit || !wantLFS {
		t.Fatalf("full archive members missing .git=%t lfs=%t", wantGit, wantLFS)
	}
}

func TestRecloneableIsMetadataOnly(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "backup")
	repository := newRecloneableRepository(t, "recloneable")
	repository.RemoteURL = "https://user:token@example.com/org/repo.git"
	manifest := Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{repository}}
	if err := Create(context.Background(), CreateOptions{Destination: destination, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	metadata, err := readBackupMetadata(filepath.Join(destination, "backup.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.Artifacts) != 0 {
		t.Fatalf("recloneable artifacts = %#v, want none", metadata.Artifacts)
	}
	if len(metadata.Manifest.Repositories) != 1 || metadata.Manifest.Repositories[0].RemoteURL != "https://example.com/org/repo.git" {
		t.Fatalf("embedded manifest = %#v", metadata.Manifest)
	}
}

func TestRecloneableRepositoryIsRevalidatedBeforeMetadataOnlyBackup(t *testing.T) {
	repository := newRecloneableRepository(t, "changed")
	if err := os.WriteFile(filepath.Join(repository.Path, "new-untracked-file"), []byte("local data"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Create(context.Background(), CreateOptions{
		Destination: filepath.Join(t.TempDir(), "backup"),
		Manifest:    Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{repository}},
	})
	if err == nil || !strings.Contains(err.Error(), "no longer clean") {
		t.Fatalf("Create() error = %v, want stale-audit rejection", err)
	}
}

func TestCreateCheckpointsBeforeFirstArtifact(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "backup")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, ".fget-backup-metadata-crash.tmp"), []byte("partial metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := CreateOptions{
		Destination: destination,
		Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{
			ID: "full", Path: repository, Classification: ClassificationFull,
		}}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Create(ctx, options); err == nil {
		t.Fatal("Create() succeeded with a canceled context")
	}
	metadata, err := readBackupMetadata(filepath.Join(destination, "backup.json"))
	if err != nil {
		t.Fatalf("initial checkpoint missing: %v", err)
	}
	if metadata.Complete {
		t.Fatal("initial checkpoint is complete")
	}
	if err := Create(context.Background(), options); err != nil {
		t.Fatalf("Create() could not resume initial checkpoint: %v", err)
	}
}

func TestRemoveStaleTempsRecognizesOnlyBackupTempFiles(t *testing.T) {
	destination := t.TempDir()
	nested := filepath.Join(destination, "repos", "hash")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := []string{
		filepath.Join(destination, ".fget-backup-metadata-crash.tmp"),
		filepath.Join(nested, ".fget-backup-artifact-crash.tmp"),
	}
	for _, path := range stale {
		if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	preserved := []string{
		filepath.Join(destination, ".fget-backup-metadata-crash"),
		filepath.Join(destination, ".fget-other-crash.tmp"),
		filepath.Join(destination, "user.tmp"),
	}
	for _, path := range preserved {
		if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	preservedDirectory := filepath.Join(destination, ".fget-backup-metadata-directory.tmp")
	if err := os.Mkdir(preservedDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := removeStaleTemps(destination); err != nil {
		t.Fatal(err)
	}
	for _, path := range stale {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale temp %q remains: %v", path, err)
		}
	}
	for _, path := range append(preserved, preservedDirectory) {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unrelated path %q removed: %v", path, err)
		}
	}
}

func TestCreateRejectsUnrelatedTempShapedEntry(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "backup")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(destination, ".fget-other-crash.tmp")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Create(context.Background(), CreateOptions{
		Destination: destination,
		Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{
			ID: "full", Path: repository, Classification: ClassificationFull,
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("Create() error = %v, want non-empty rejection", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated entry changed: %v", err)
	}
}
