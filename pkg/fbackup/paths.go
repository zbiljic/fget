package fbackup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func rejectSymlinkRoot(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("backup destination is a symlink: %q", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("backup destination is not a directory: %q", path)
	}
	return nil
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path = abs
	var suffix []string
	for {
		_, err := os.Lstat(path)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return "", err
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path, nil
		}
		suffix = append(suffix, filepath.Base(path))
		path = parent
	}
}

func validateSourceDestinationOverlap(destination string, repositories []RepositoryEntry) error {
	destinationCanonical, err := canonicalPath(destination)
	if err != nil {
		return fmt.Errorf("canonicalize backup destination: %w", err)
	}
	for _, repo := range repositories {
		if repo.Path == "" {
			continue
		}
		repoCanonical, err := canonicalPath(repo.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("canonicalize repository %q: %w", repo.ID, err)
		}
		rel, err := filepath.Rel(repoCanonical, destinationCanonical)
		if err != nil {
			return err
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return fmt.Errorf("backup destination overlaps repository %q", repo.ID)
		}
		rel, err = filepath.Rel(destinationCanonical, repoCanonical)
		if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
			return fmt.Errorf("backup destination overlaps repository %q", repo.ID)
		}
	}
	return nil
}

func ensureSafeDirectory(root, relative string) error {
	if relative == "" || relative == "." {
		return nil
	}
	if err := validateArtifactPath(relative); err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o755); err != nil {
				if !errors.Is(err, os.ErrExist) {
					return err
				}
				info, err = os.Lstat(current)
				if err != nil {
					return err
				}
			} else {
				continue
			}
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup path component %q is a symlink", filepath.ToSlash(filepath.Join(relative, part)))
		}
		if !info.IsDir() {
			return fmt.Errorf("backup path component %q is not a directory", current)
		}
	}
	return nil
}

func ensureSafeArtifactPath(root, relative string) (string, error) {
	if err := validateArtifactPath(relative); err != nil {
		return "", err
	}
	parts := strings.Split(filepath.FromSlash(relative), string(filepath.Separator))
	current := root
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("backup path component %q is a symlink", current)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("backup path component %q is not a directory", current)
		}
	}
	return filepath.Join(root, filepath.FromSlash(relative)), nil
}

func validateRestoreDestination(destination string, entries []RepositoryEntry, dryRun bool) error {
	if err := rejectRestoreSymlinkAncestors(destination); err != nil {
		return err
	}
	info, err := os.Lstat(destination)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("restore destination is not a directory")
		}
		contents, readErr := os.ReadDir(destination)
		if readErr != nil {
			return readErr
		}
		if len(contents) != 0 {
			return errors.New("restore destination is not empty")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	seen := map[string]bool{}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.ID == "" || filepath.IsAbs(entry.ID) || filepath.VolumeName(entry.ID) != "" || !safeRelativePath(filepath.ToSlash(entry.ID)) {
			return fmt.Errorf("unsafe repository ID %q", entry.ID)
		}
		key := strings.ToLower(filepath.Clean(entry.ID))
		if seen[key] {
			return fmt.Errorf("repository ID collision %q", entry.ID)
		}
		seen[key] = true
		clean := strings.ToLower(filepath.ToSlash(filepath.Clean(entry.ID)))
		for _, previous := range paths {
			if clean == previous || strings.HasPrefix(clean, previous+"/") || strings.HasPrefix(previous, clean+"/") {
				return fmt.Errorf("repository ID ancestor collision between %q and %q", entry.ID, previous)
			}
		}
		paths = append(paths, clean)
	}
	if dryRun {
		return nil
	}
	for _, entry := range entries {
		if err := ensureSafeDirectory(destination, filepath.ToSlash(filepath.Dir(entry.ID))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func rejectRestoreSymlinkAncestors(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for current := path; ; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		if statErr == nil {
			// macOS exposes temporary directories through the conventional /var
			// symlink; it is an OS-owned ancestor rather than a restore redirect.
			if info.Mode()&os.ModeSymlink != 0 && current != string(filepath.Separator)+"var" && current != string(filepath.Separator)+"tmp" {
				return fmt.Errorf("restore destination ancestor is a symlink: %q", current)
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}

func validateResumeDestination(destination string, state restoreState) error {
	completed := make(map[string]bool, len(state.Completed))
	for _, id := range state.Completed {
		completed[filepath.ToSlash(filepath.Clean(id))] = true
	}
	return filepath.WalkDir(destination, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || path == destination {
			return walkErr
		}
		rel, err := filepath.Rel(destination, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == restoreStateFile {
			return nil
		}
		for id := range completed {
			switch {
			case rel == id:
				if !entry.IsDir() {
					return errors.New("restore destination contains an invalid repository path")
				}
				return nil
			case strings.HasPrefix(rel, id+"/"):
				return nil
			case strings.HasPrefix(id, rel+"/"):
				if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
					return errors.New("restore destination contains an unsafe repository ancestor")
				}
				return nil
			}
		}
		return errors.New("restore destination contains unrelated data")
	})
}

func cleanupEmptyRestoreAncestors(root, path string) {
	for path != root && path != "." {
		if err := os.Remove(path); err != nil {
			return
		}
		path = filepath.Dir(path)
	}
}
