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
	"sort"
	"strings"

	"github.com/zbiljic/fget/pkg/gitinspect"
)

func runGitToFile(ctx context.Context, runner gitinspect.StreamingRunner, repoPath, output string, args ...string) error {
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = runner.RunTo(ctx, repoPath, f, args...)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func writeUntrackedTar(ctx context.Context, runner gitinspect.Runner, repoPath, output string) error {
	out, err := runner.Run(ctx, repoPath, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return err
	}
	var paths []string
	for _, p := range strings.Split(out.Stdout, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return writeTar(ctx, repoPath, output, paths)
}

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

func extractTar(path, root string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	directoryModes := map[string]os.FileMode{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(h.Name)
		if !safeRelativePath(name) {
			return fmt.Errorf("unsafe tar member path %q", h.Name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := ensureSafeDirectory(root, filepath.ToSlash(filepath.Dir(name))); err != nil {
			return err
		}
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("tar member already exists: %q", name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.Mkdir(target, os.FileMode(h.Mode)|0o700); err != nil {
				return err
			}
			directoryModes[target] = os.FileMode(h.Mode)
		case tar.TypeReg:
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(h.Mode))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(h.Linkname) || filepath.VolumeName(h.Linkname) != "" {
				return fmt.Errorf("unsafe tar symlink %q", h.Linkname)
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(name), filepath.FromSlash(h.Linkname)))
			if !safeRelativePath(filepath.ToSlash(resolved)) {
				return fmt.Errorf("unsafe tar symlink %q", h.Linkname)
			}
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported tar member type %d", h.Typeflag)
		}
	}

	paths := make([]string, 0, len(directoryModes))
	for directory := range directoryModes {
		paths = append(paths, directory)
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, directory := range paths {
		if err := os.Chmod(directory, directoryModes[directory]); err != nil {
			return err
		}
	}
	return nil
}
