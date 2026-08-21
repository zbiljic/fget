package fbackup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyRejectsCorruptArtifactAndCreateDoesNotRepairCompleteBackup(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "data"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{
		ID: "full", Path: repository, Classification: ClassificationFull,
	}}}
	destination := filepath.Join(root, "backup")
	options := CreateOptions{Destination: destination, Manifest: manifest}
	if err := Create(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	metadataBefore, err := os.ReadFile(filepath.Join(destination, "backup.json"))
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(destination, "repos", repositoryDirName("full"), "full.tar.gz")
	if file, err := os.OpenFile(artifact, os.O_APPEND|os.O_WRONLY, 0); err != nil {
		t.Fatal(err)
	} else {
		_, _ = file.Write([]byte("corrupt"))
		_ = file.Close()
	}
	if err := Verify(context.Background(), destination, false); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("Verify() error = %v, want size mismatch", err)
	}
	if err := Create(context.Background(), options); err == nil {
		t.Fatal("Create() repaired an immutable completed backup")
	}
	metadataAfter, err := os.ReadFile(filepath.Join(destination, "backup.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(metadataBefore) != string(metadataAfter) {
		t.Fatal("Create() rewrote completed backup metadata")
	}
}

func TestVerifyIgnoresUnindexedTopLevelFiles(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "backup")
	manifest := Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{
		ID: "full", Path: repository, Classification: ClassificationFull,
	}}}
	if err := Create(context.Background(), CreateOptions{Destination: destination, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, ".DS_Store"), []byte("incidental"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), destination, false); err != nil {
		t.Fatalf("Verify() rejected an unindexed top-level file: %v", err)
	}
}

func TestVerifyRejectsInvalidArtifactOwnership(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*BackupMetadata)
		message string
	}{
		{
			name: "ownerless artifact",
			mutate: func(metadata *BackupMetadata) {
				metadata.Artifacts = append(metadata.Artifacts, ArtifactRecord{
					RepositoryID: "missing", Kind: "full", Path: "repos/missing/full.tar.gz", SHA256: "digest",
				})
			},
			message: "no matching repository manifest entry",
		},
		{
			name: "duplicate logical kind",
			mutate: func(metadata *BackupMetadata) {
				metadata.Artifacts = append(metadata.Artifacts, metadata.Artifacts[0])
			},
			message: "duplicate artifact kind",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repository := filepath.Join(t.TempDir(), "repository")
			if err := os.Mkdir(repository, 0o755); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(t.TempDir(), "backup")
			manifest := Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{
				ID: "full", Path: repository, Classification: ClassificationFull,
			}}}
			if err := Create(context.Background(), CreateOptions{Destination: destination, Manifest: manifest}); err != nil {
				t.Fatal(err)
			}
			metadata, err := readBackupMetadata(filepath.Join(destination, "backup.json"))
			if err != nil {
				t.Fatal(err)
			}
			testCase.mutate(&metadata)
			if err := writeJSONAtomic(filepath.Join(destination, "backup.json"), metadata, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := Verify(context.Background(), destination, false); err == nil || !strings.Contains(err.Error(), testCase.message) {
				t.Fatalf("Verify() error = %v, want %q", err, testCase.message)
			}
		})
	}
}
