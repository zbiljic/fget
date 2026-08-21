package fbackup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func writeFullTar(ctx context.Context, repoPath, output string) error {
	var paths []string
	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(repoPath, path)
		if err != nil {
			return err
		}
		if rel != "." {
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(paths)
	return writeTar(ctx, repoPath, output, paths)
}

func writeTar(ctx context.Context, root, output string, paths []string) error {
	paths = append([]string(nil), paths...)
	sort.Strings(paths)
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, rel := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !safeRelativePath(rel) {
			return fmt.Errorf("unsafe tar path %q", rel)
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(full)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.Mode()&os.ModeSymlink != 0 {
			hdr.Linkname, err = os.Readlink(full)
			if err != nil {
				return err
			}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			in, err := os.Open(full)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(tw, in)
			closeErr := in.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Sync()
}
