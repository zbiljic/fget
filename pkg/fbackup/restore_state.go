package fbackup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const restoreStateFile = ".fget.restore-state.json"

type restoreState struct {
	SchemaVersion   string            `json:"schema_version"`
	Backup          string            `json:"backup"`
	SourceAuditHash string            `json:"source_audit_hash"`
	Completed       []string          `json:"completed"`
	Failed          map[string]string `json:"failed,omitempty"`
	Fingerprints    map[string]string `json:"fingerprints,omitempty"`
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fingerprintRepository(root string) (string, error) {
	var fingerprint strings.Builder
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		kind := "other"
		content := ""
		switch {
		case info.IsDir():
			kind = "dir"
		case info.Mode()&os.ModeSymlink != 0:
			kind = "symlink"
			content, err = os.Readlink(path)
			if err != nil {
				return err
			}
		case info.Mode().IsRegular():
			kind = "file"
			_, content, err = hashFile(path)
			if err != nil {
				return err
			}
		}
		fmt.Fprintf(&fingerprint, "%s\x00%s\x00%o\x00%s\x00%s\n", rel, kind, info.Mode().Perm(), content, info.Mode().String())
		return nil
	})
	if err != nil {
		return "", err
	}
	return hashBytes([]byte(fingerprint.String())), nil
}

func readRestoreState(destination, backup string, metadata BackupMetadata) (restoreState, error) {
	path := filepath.Join(destination, restoreStateFile)
	var state restoreState
	if err := rejectSymlinkOrNonRegular(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return restoreState{SchemaVersion: BackupSchemaVersion, Backup: backup, SourceAuditHash: metadata.SourceAuditHash}, nil
		}
		return state, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("read restore state: %w", err)
	}
	if state.SchemaVersion != BackupSchemaVersion || state.Backup != backup || state.SourceAuditHash != metadata.SourceAuditHash {
		return state, errors.New("restore state belongs to a different backup")
	}
	return state, nil
}
