package fbackup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zbiljic/fget/pkg/gitinspect"
)

func Verify(ctx context.Context, destination string, deep bool) error {
	return verifyWithRunner(ctx, destination, deep, gitinspect.CLIRunner{})
}

func verifyWithRunner(ctx context.Context, destination string, deep bool, runner gitinspect.Runner) error {
	destination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if err := rejectSymlinkRoot(destination); err != nil {
		return err
	}
	metadata, err := readBackupMetadata(filepath.Join(destination, "backup.json"))
	if err != nil {
		return err
	}
	if !metadata.Complete {
		return errors.New("backup is incomplete")
	}
	return verifyArtifacts(ctx, destination, metadata, deep, runner)
}

func verifyArtifacts(ctx context.Context, destination string, metadata BackupMetadata, deep bool, runner gitinspect.Runner) error {
	if metadata.Manifest.Version != ManifestVersion {
		return fmt.Errorf("unsupported embedded manifest version %q", metadata.Manifest.Version)
	}
	auditHash, err := manifestHash(metadata.Manifest)
	if err != nil {
		return err
	}
	if metadata.SourceAuditHash == "" || metadata.SourceAuditHash != auditHash {
		return errors.New("backup audit hash mismatch")
	}

	repositories, err := expectedRepositoryArtifacts(metadata.Manifest.Repositories)
	if err != nil {
		return err
	}
	seenKeys := make(map[string]bool, len(metadata.Artifacts))
	seenPaths := make(map[string]bool, len(metadata.Artifacts))
	for _, record := range metadata.Artifacts {
		if record.RepositoryID == "" || record.Kind == "" || record.Path == "" || record.SHA256 == "" {
			return errors.New("artifact record has an empty required field")
		}
		key := artifactKey(record.RepositoryID, record.Kind)
		if seenKeys[key] {
			return fmt.Errorf("duplicate artifact kind %q for repository %q", record.Kind, record.RepositoryID)
		}
		seenKeys[key] = true

		expectedKinds, ok := repositories[record.RepositoryID]
		if !ok || !expectedKinds[record.Kind] {
			return fmt.Errorf("artifact %q has no matching repository manifest entry", record.RepositoryID)
		}
		expectedDirectory := filepath.ToSlash(filepath.Join("repos", repositoryDirName(record.RepositoryID)))
		if !strings.HasPrefix(filepath.ToSlash(record.Path), expectedDirectory+"/") {
			return fmt.Errorf("artifact %q is outside its repository directory", record.RepositoryID)
		}
		if err := validateArtifactPath(record.Path); err != nil {
			return err
		}
		pathKey := filepath.ToSlash(record.Path)
		if seenPaths[pathKey] {
			return fmt.Errorf("duplicate artifact path %q", record.Path)
		}
		seenPaths[pathKey] = true
		if err := verifyRecord(destination, record); err != nil {
			return err
		}
		if deep {
			if err := verifyDeepArtifact(ctx, runner, destination, record); err != nil {
				return err
			}
		}
	}
	for repositoryID, kinds := range repositories {
		for kind := range kinds {
			if !seenKeys[artifactKey(repositoryID, kind)] {
				return fmt.Errorf("repository %q is missing artifact kind %q", repositoryID, kind)
			}
		}
	}
	return nil
}

func expectedRepositoryArtifacts(entries []RepositoryEntry) (map[string]map[string]bool, error) {
	result := make(map[string]map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.ID == "" || entry.Classification == ClassificationProblem || entry.Classification == ClassificationUnknown {
			return nil, fmt.Errorf("invalid repository manifest entry %q", entry.ID)
		}
		if _, exists := result[entry.ID]; exists {
			return nil, fmt.Errorf("duplicate repository manifest entry %q", entry.ID)
		}
		switch entry.Classification {
		case ClassificationRecloneable, ClassificationFull:
		default:
			return nil, fmt.Errorf("unsupported repository classification %q", entry.Classification)
		}
		kinds := make(map[string]bool)
		for _, kind := range expectedArtifactKinds(entry.Classification) {
			kinds[kind] = true
		}
		result[entry.ID] = kinds
	}
	return result, nil
}

func verifyRecord(destination string, record ArtifactRecord) error {
	if err := validateArtifactPath(record.Path); err != nil {
		return err
	}
	path, err := ensureSafeArtifactPath(destination, record.Path)
	if err != nil {
		return fmt.Errorf("artifact %q: %w", record.RepositoryID, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("artifact %q: %w", record.RepositoryID, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("artifact %q is not a regular file", record.RepositoryID)
	}
	if info.Size() != record.Size {
		return fmt.Errorf("artifact %q size mismatch", record.RepositoryID)
	}
	_, digest, err := hashFile(path)
	if err != nil {
		return fmt.Errorf("artifact %q: %w", record.RepositoryID, err)
	}
	if !strings.EqualFold(digest, record.SHA256) {
		return fmt.Errorf("artifact %q checksum mismatch", record.RepositoryID)
	}
	return nil
}

func verifyDeepArtifact(ctx context.Context, runner gitinspect.Runner, destination string, record ArtifactRecord) error {
	path := filepath.Join(destination, filepath.FromSlash(record.Path))
	switch record.Kind {
	case "full":
		if err := verifyTar(ctx, path); err != nil {
			return fmt.Errorf("artifact %q tar verification failed: %w", record.RepositoryID, err)
		}
	}
	return ctx.Err()
}

func verifyTar(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()
	tarReader := tar.NewReader(gzipReader)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		if !safeRelativePath(filepath.ToSlash(header.Name)) {
			return fmt.Errorf("unsafe tar member path %q", header.Name)
		}
		if _, err := io.Copy(io.Discard, tarReader); err != nil {
			return err
		}
	}
	return nil
}
