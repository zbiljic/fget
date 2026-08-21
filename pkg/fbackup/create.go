package fbackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/zbiljic/fget/pkg/gitinspect"
)

type CreateOptions struct {
	Destination string
	Manifest    Manifest
	Progress    func(repositoryID, status string)
	GitRunner   gitinspect.StreamingRunner
}

func Create(ctx context.Context, options CreateOptions) error {
	options, destination, auditHash, err := prepareCreateOptions(options)
	if err != nil {
		return err
	}

	metadataPath := filepath.Join(destination, "backup.json")
	metadata, err := openOrInitialize(destination, options.Manifest, auditHash)
	if err != nil {
		return err
	}
	if metadata.Complete {
		return verifyWithRunner(ctx, destination, false, options.GitRunner)
	}

	// Publish the initial checkpoint before any artifact work. A failure on the
	// first repository can therefore be retried in the same destination.
	if err := writeJSONAtomic(metadataPath, metadata, 0o644); err != nil {
		return err
	}
	if err := removeStaleTemps(destination); err != nil {
		return err
	}

	for _, repository := range options.Manifest.Repositories {
		if err := ctx.Err(); err != nil {
			return err
		}
		if options.Progress != nil {
			options.Progress(repository.ID, "starting")
		}
		if err := validateMetadataOnlyRepository(ctx, repository, options.GitRunner); err != nil {
			return err
		}
		if err := createRepository(ctx, destination, repository, &metadata, options.GitRunner, options.Progress); err != nil {
			return err
		}
	}

	if err := verifyArtifacts(ctx, destination, metadata, false, options.GitRunner); err != nil {
		return err
	}
	metadata.Complete = true
	return writeJSONAtomic(metadataPath, metadata, 0o644)
}

func prepareCreateOptions(options CreateOptions) (CreateOptions, string, string, error) {
	if options.Destination == "" {
		return options, "", "", errors.New("backup destination is required")
	}
	if options.GitRunner == nil {
		options.GitRunner = gitinspect.CLIRunner{}
	}
	if options.Manifest.Version != ManifestVersion {
		return options, "", "", fmt.Errorf("unsupported manifest version %q", options.Manifest.Version)
	}
	options.Manifest = snapshotManifest(options.Manifest)
	auditHash, err := manifestHash(options.Manifest)
	if err != nil {
		return options, "", "", err
	}
	destination, err := filepath.Abs(options.Destination)
	if err != nil {
		return options, "", "", err
	}
	if err := validateSourceDestinationOverlap(destination, options.Manifest.Repositories); err != nil {
		return options, "", "", err
	}
	if err := rejectSymlinkRoot(destination); err != nil {
		return options, "", "", err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return options, "", "", err
	}
	seenRepositories := make(map[string]bool)
	for _, repository := range options.Manifest.Repositories {
		if repository.ID == "" || seenRepositories[repository.ID] {
			return options, "", "", fmt.Errorf("manifest contains duplicate or empty repository ID %q", repository.ID)
		}
		seenRepositories[repository.ID] = true
		switch repository.Classification {
		case ClassificationRecloneable:
			if repository.RemoteState != RemoteStateReachable {
				return options, "", "", fmt.Errorf("recloneable repository %q does not have a verified remote", repository.ID)
			}
		case ClassificationFull:
		default:
			return options, "", "", fmt.Errorf("repository %q has unsupported classification %q", repository.ID, repository.Classification)
		}
	}
	return options, destination, auditHash, nil
}

func openOrInitialize(destination string, manifest Manifest, auditHash string) (BackupMetadata, error) {
	metadataPath := filepath.Join(destination, "backup.json")
	metadata, metadataErr := readBackupMetadata(metadataPath)
	if metadataErr == nil {
		if metadata.SourceAuditHash != auditHash {
			return metadata, errors.New("backup destination belongs to a different audit manifest")
		}
		return metadata, nil
	}
	if !errors.Is(metadataErr, os.ErrNotExist) {
		return metadata, metadataErr
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		return metadata, err
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), ".fbackup-") && strings.HasSuffix(entry.Name(), ".tmp") {
			if err := os.Remove(filepath.Join(destination, entry.Name())); err != nil {
				return metadata, err
			}
			continue
		}
		return metadata, errors.New("backup destination is not empty")
	}
	return BackupMetadata{
		SchemaVersion:   BackupSchemaVersion,
		SourceAuditHash: auditHash,
		CreatedAt:       time.Now().UTC(),
		Manifest:        manifest,
	}, nil
}

func repositoryDirName(id string) string {
	hash := sha256.Sum256([]byte(id))
	return hex.EncodeToString(hash[:])
}

func createRepository(
	ctx context.Context,
	destination string,
	repository RepositoryEntry,
	metadata *BackupMetadata,
	runner gitinspect.StreamingRunner,
	progress func(string, string),
) error {
	if repository.Classification == ClassificationRecloneable {
		if progress != nil {
			progress(repository.ID, "complete")
		}
		return nil
	}

	base := filepath.Join("repos", repositoryDirName(repository.ID))
	if err := ensureSafeDirectory(destination, base); err != nil {
		return err
	}
	specifications := []struct{ kind, name string }{}
	switch repository.Classification {
	case ClassificationFull:
		specifications = append(specifications, struct{ kind, name string }{"full", "full.tar.gz"})
	}

	for _, specification := range specifications {
		if err := ctx.Err(); err != nil {
			return err
		}
		if previous, ok := findArtifact(metadata.Artifacts, repository.ID, specification.kind); ok && verifyRecord(destination, previous) == nil {
			continue
		}

		relativePath := filepath.ToSlash(filepath.Join(base, specification.name))
		destinationPath := filepath.Join(destination, filepath.FromSlash(relativePath))
		temporary, err := os.CreateTemp(filepath.Dir(destinationPath), ".artifact-*.tmp")
		if err != nil {
			return err
		}
		temporaryPath := temporary.Name()
		_ = temporary.Close()

		var writeErr error
		switch specification.kind {
		case "full":
			writeErr = writeFullTar(ctx, repository.Path, temporaryPath)
		}
		if writeErr != nil {
			if ctx.Err() == nil {
				_ = os.Remove(temporaryPath)
			}
			return fmt.Errorf("create %s for %q: %w", specification.kind, repository.ID, writeErr)
		}
		size, digest, err := hashFile(temporaryPath)
		if err != nil {
			_ = os.Remove(temporaryPath)
			return err
		}
		if err := os.Rename(temporaryPath, destinationPath); err != nil {
			_ = os.Remove(temporaryPath)
			return err
		}
		record := ArtifactRecord{
			RepositoryID: repository.ID,
			Kind:         specification.kind,
			Path:         relativePath,
			Size:         size,
			SHA256:       digest,
		}
		upsertArtifact(metadata, record)
		if err := writeJSONAtomic(filepath.Join(destination, "backup.json"), metadata, 0o644); err != nil {
			return err
		}
		if progress != nil {
			progress(repository.ID, specification.kind)
		}
	}
	if progress != nil {
		progress(repository.ID, "complete")
	}
	return nil
}

func findArtifact(records []ArtifactRecord, repositoryID, kind string) (ArtifactRecord, bool) {
	for _, record := range records {
		if record.RepositoryID == repositoryID && record.Kind == kind {
			return record, true
		}
	}
	return ArtifactRecord{}, false
}

func validateMetadataOnlyRepository(ctx context.Context, repository RepositoryEntry, runner gitinspect.Runner) error {
	if repository.Classification != ClassificationRecloneable {
		return nil
	}
	state, err := gitinspect.InspectState(ctx, repository.Path, runner)
	if err != nil {
		return fmt.Errorf("revalidate repository %q: %w", repository.ID, err)
	}
	if !reflect.DeepEqual(state, repository.Git) {
		return fmt.Errorf("repository %q changed since audit", repository.ID)
	}
	status, err := runner.Run(ctx, repository.Path, "status", "--porcelain=v1", "-z")
	if err != nil {
		return fmt.Errorf("revalidate repository %q worktree: %w", repository.ID, err)
	}
	if status.Stdout != "" {
		return fmt.Errorf("repository %q is no longer clean; run backup audit again", repository.ID)
	}
	if _, err := runner.Run(ctx, repository.Path, "ls-remote", "--symref", "origin"); err != nil {
		return fmt.Errorf("revalidate repository %q remote: %w", repository.ID, err)
	}
	return nil
}

func removeStaleTemps(destination string) error {
	return filepath.WalkDir(destination, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if (strings.HasPrefix(name, ".artifact-") || strings.HasPrefix(name, ".fbackup-")) && strings.HasSuffix(name, ".tmp") {
			return os.Remove(path)
		}
		return nil
	})
}
