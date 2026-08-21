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
