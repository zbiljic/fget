package fbackup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zbiljic/fget/pkg/giturl"
)

const BackupSchemaVersion = "1"

var errInvalidArtifactPath = errors.New("invalid backup artifact path")

// ArtifactRecord indexes one atomically published file in a backup. A record
// is added only after the file has been closed, renamed, sized, and hashed.
type ArtifactRecord struct {
	RepositoryID string `json:"repository_id"`
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
}

// BackupMetadata is both the resumable checkpoint and the final backup index.
// It is rewritten atomically while Complete is false and immutable afterward.
type BackupMetadata struct {
	SchemaVersion   string           `json:"schema_version"`
	SourceAuditHash string           `json:"source_audit_hash"`
	CreatedAt       time.Time        `json:"created_at"`
	Complete        bool             `json:"complete"`
	Manifest        Manifest         `json:"manifest"`
	Artifacts       []ArtifactRecord `json:"artifacts"`
}

func snapshotManifest(manifest Manifest) Manifest {
	snapshot := manifest
	snapshot.Roots = append([]string(nil), manifest.Roots...)
	if manifest.Catalog != nil {
		catalog := *manifest.Catalog
		snapshot.Catalog = &catalog
	}
	snapshot.Repositories = make([]RepositoryEntry, len(manifest.Repositories))
	for index, repository := range manifest.Repositories {
		repository.RemoteURL = giturl.Sanitize(repository.RemoteURL)
		repository.ReasonCodes = append([]string(nil), repository.ReasonCodes...)
		repository.Errors = append([]RepositoryError(nil), repository.Errors...)
		if repository.Git.Upstream != nil {
			upstream := *repository.Git.Upstream
			repository.Git.Upstream = &upstream
		}
		snapshot.Repositories[index] = repository
	}
	return snapshot
}

func manifestHash(manifest Manifest) (string, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func expectedArtifactKinds(classification Classification) []string {
	switch classification {
	case ClassificationRecloneable:
		return nil
	case ClassificationFull:
		return []string{"full"}
	default:
		return nil
	}
}

func artifactKey(repositoryID, kind string) string { return repositoryID + "\x00" + kind }

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".fbackup-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer func() { _ = file.Close(); _ = os.Remove(temporaryPath) }()
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	if directory, err := os.Open(filepath.Dir(path)); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func readBackupMetadata(path string) (BackupMetadata, error) {
	var metadata BackupMetadata
	if err := rejectSymlinkOrNonRegular(path); err != nil {
		return metadata, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return metadata, err
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return metadata, fmt.Errorf("read backup metadata: %w", err)
	}
	if metadata.SchemaVersion != BackupSchemaVersion {
		return metadata, fmt.Errorf("unsupported backup schema version %q", metadata.SchemaVersion)
	}
	return metadata, nil
}

func rejectSymlinkOrNonRegular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("backup file %q is a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("backup file %q is not regular", path)
	}
	return nil
}

func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func hashFile(path string) (size int64, digest string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	size, err = io.Copy(hash, file)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

func safeRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return false
	}
	clean := filepath.Clean(filepath.ToSlash(path))
	return clean == filepath.ToSlash(path) && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.Contains(clean, ":")
}

func validateArtifactPath(path string) error {
	if !safeRelativePath(path) {
		return fmt.Errorf("%w: %q", errInvalidArtifactPath, path)
	}
	return nil
}

func upsertArtifact(metadata *BackupMetadata, record ArtifactRecord) {
	for index := range metadata.Artifacts {
		if metadata.Artifacts[index].RepositoryID == record.RepositoryID && metadata.Artifacts[index].Kind == record.Kind {
			metadata.Artifacts[index] = record
			return
		}
	}
	metadata.Artifacts = append(metadata.Artifacts, record)
	sort.Slice(metadata.Artifacts, func(i, j int) bool {
		if metadata.Artifacts[i].RepositoryID == metadata.Artifacts[j].RepositoryID {
			return metadata.Artifacts[i].Kind < metadata.Artifacts[j].Kind
		}
		return metadata.Artifacts[i].RepositoryID < metadata.Artifacts[j].RepositoryID
	})
}
