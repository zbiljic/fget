package fbackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateVerifyDeltaAndResume(t *testing.T) { //nolint:gocyclo // integration coverage intentionally exercises all delta artifacts
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mustGitTest(t, root, "init", "-q", repo)
	mustGitTest(t, repo, "config", "user.email", "test@example.com")
	mustGitTest(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repo, "add", "tracked")
	mustGitTest(t, repo, "commit", "-qm", "initial")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "untracked"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "binary"), bytes.Repeat([]byte{0}, 256), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repo, "add", "binary")
	modifiedBinary := append([]byte{1}, bytes.Repeat([]byte{0}, 255)...)
	if err := os.WriteFile(filepath.Join(repo, "binary"), modifiedBinary, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "ignored"), []byte("must not backup"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repo, "branch", "local-only")
	mustGitTest(t, repo, "tag", "local-tag")

	destination := filepath.Join(root, "backup")
	manifest := Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{ID: "example/repo", Path: repo, RemoteURL: "https://user:secret@example.com/acme/repo.git", Classification: ClassificationDelta}}}
	if err := Create(context.Background(), CreateOptions{Destination: destination, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), destination, true); err != nil {
		t.Fatal(err)
	}
	metadata, err := readBackupMetadata(filepath.Join(destination, "backup.json"))
	if err != nil || len(metadata.Manifest.Repositories) != 1 {
		t.Fatalf("embedded manifest = %#v, error=%v", metadata.Manifest, err)
	}
	if metadata.Manifest.Repositories[0].RemoteURL != "https://example.com/acme/repo.git" {
		t.Fatalf("repository manifest entry = %#v", metadata.Manifest.Repositories[0])
	}
	bundleHeads := exec.Command("git", "bundle", "list-heads", filepath.Join(destination, "repos", repositoryDirName("example/repo"), "repository.bundle"))
	heads, err := bundleHeads.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(heads, []byte("refs/heads/local-only")) || !bytes.Contains(heads, []byte("refs/tags/local-tag")) {
		t.Fatalf("bundle heads missing local refs: %s", heads)
	}
	artifactRoot := filepath.Join(destination, "repos", repositoryDirName("example/repo"))
	for _, name := range []string{"repository.bundle", "tracked.patch", "untracked.tar.gz"} {
		artifactPath := filepath.Join(artifactRoot, name)
		original, err := os.ReadFile(artifactPath)
		if err != nil {
			t.Fatal(err)
		}
		corrupt := append([]byte(nil), original...)
		corrupt[0] ^= 0xff
		if err := os.WriteFile(artifactPath, corrupt, 0o600); err != nil {
			t.Fatal(err)
		}
		err = Verify(context.Background(), destination, false)
		if err == nil || !strings.Contains(err.Error(), "example/repo") {
			t.Fatalf("Verify() for corrupt %s error = %v", name, err)
		}
		if err := os.WriteFile(artifactPath, original, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	patch, err := os.ReadFile(filepath.Join(destination, "repos", repositoryDirName("example/repo"), "tracked.patch"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(patch, []byte("GIT binary patch")) {
		t.Fatal("tracked.patch does not contain binary staged+unstaged patch")
	}
	untrackedArchive := filepath.Join(destination, "repos", repositoryDirName("example/repo"), "untracked.tar.gz")
	archiveFile, err := os.Open(untrackedArchive)
	if err != nil {
		t.Fatal(err)
	}
	archiveGzip, err := gzip.NewReader(archiveFile)
	if err != nil {
		_ = archiveFile.Close()
		t.Fatal(err)
	}
	archiveTar := tar.NewReader(archiveGzip)
	seenUntracked := false
	for {
		h, nextErr := archiveTar.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			_ = archiveGzip.Close()
			_ = archiveFile.Close()
			t.Fatal(nextErr)
		}
		if h.Name == "ignored" {
			_ = archiveGzip.Close()
			_ = archiveFile.Close()
			t.Fatal("ignored file entered untracked archive")
		}
		if h.Name == "untracked" {
			seenUntracked = true
		}
		if _, err := io.Copy(io.Discard, archiveTar); err != nil {
			_ = archiveGzip.Close()
			_ = archiveFile.Close()
			t.Fatal(err)
		}
	}
	_ = archiveGzip.Close()
	_ = archiveFile.Close()
	if !seenUntracked {
		t.Fatal("non-ignored untracked file missing from archive")
	}
	before, err := os.ReadFile(filepath.Join(destination, "backup.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Create(context.Background(), CreateOptions{Destination: destination, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(destination, "backup.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("resume rewrote completed metadata")
	}
}

func TestDeltaSeparatesIndexAndWorktreeLayers(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mustGitTest(t, root, "init", "-q", repo)
	mustGitTest(t, repo, "config", "user.email", "test@example.com")
	mustGitTest(t, repo, "config", "user.name", "Test")
	layerPath := filepath.Join(repo, "layer.txt")
	if err := os.WriteFile(layerPath, []byte("A\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repo, "add", "layer.txt")
	mustGitTest(t, repo, "commit", "-qm", "initial")
	if err := os.WriteFile(layerPath, []byte("B\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repo, "add", "layer.txt")
	if err := os.WriteFile(layerPath, []byte("A\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{ID: "layers", Path: repo, Classification: ClassificationDelta}}}
	destination := filepath.Join(root, "backup")
	if err := Create(context.Background(), CreateOptions{Destination: destination, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(destination, "repos", repositoryDirName("layers"))
	indexPatch, err := os.ReadFile(filepath.Join(base, "index.patch"))
	if err != nil {
		t.Fatal(err)
	}
	trackedPatch, err := os.ReadFile(filepath.Join(base, "tracked.patch"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(indexPatch, []byte("+B")) || !bytes.Contains(trackedPatch, []byte("-B")) || !bytes.Contains(trackedPatch, []byte("+A")) {
		t.Fatalf("index.patch/tracked.patch did not preserve layers: index=%q tracked=%q", indexPatch, trackedPatch)
	}
	if err := Verify(context.Background(), destination, true); err != nil {
		t.Fatal(err)
	}
}

func TestCreateCancellationResumesCompletedArtifacts(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mustGitTest(t, root, "init", "-q", repo)
	mustGitTest(t, repo, "config", "user.email", "test@example.com")
	mustGitTest(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repo, "add", "tracked")
	mustGitTest(t, repo, "commit", "-qm", "initial")
	manifest := Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{ID: "example/repo", Path: repo, Classification: ClassificationDelta}}}
	destination := filepath.Join(root, "backup")
	ctx, cancel := context.WithCancel(context.Background())
	if err := Create(ctx, CreateOptions{Destination: destination, Manifest: manifest, Progress: func(_, status string) {
		if status == "bundle" {
			cancel()
		}
	}}); err == nil {
		t.Fatal("Create() succeeded after cancellation")
	}
	bundle := filepath.Join(destination, "repos", repositoryDirName("example/repo"), "repository.bundle")
	before, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := Create(context.Background(), CreateOptions{Destination: destination, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("resume rewrote completed bundle")
	}
	if err := Verify(context.Background(), destination, true); err != nil {
		t.Fatal(err)
	}
}

func TestCreateMultipleArtifactDirectories(t *testing.T) {
	root := t.TempDir()
	repositories := make([]RepositoryEntry, 4)
	for i := range repositories {
		repo := filepath.Join(root, "repo-"+string(rune('a'+i)))
		mustGitTest(t, root, "init", "-q", repo)
		mustGitTest(t, repo, "config", "user.email", "test@example.com")
		mustGitTest(t, repo, "config", "user.name", "Test")
		if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}
		mustGitTest(t, repo, "add", "tracked")
		mustGitTest(t, repo, "commit", "-qm", "initial")
		classification := ClassificationFull
		if i%2 == 0 {
			classification = ClassificationDelta
			if err := os.WriteFile(filepath.Join(repo, "dirty"), []byte("dirty"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		repositories[i] = RepositoryEntry{ID: "repo-" + string(rune('a'+i)), Path: repo, Classification: classification}
	}
	destination := filepath.Join(root, "backup")
	manifest := Manifest{Version: ManifestVersion, Repositories: repositories}
	if err := Create(context.Background(), CreateOptions{Destination: destination, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), destination, true); err != nil {
		t.Fatal(err)
	}
}
