package fbackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTarRejectsCorruptAndEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "unsafe.tar.gz")
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../../outside"}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTar(archivePath, filepath.Join(root, "extract")); err == nil {
		t.Fatal("escaping symlink accepted")
	}
	absolutePath := filepath.Join(root, "absolute.tar.gz")
	data.Reset()
	gz = gzip.NewWriter(&data)
	tw = tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "absolute", Typeflag: tar.TypeSymlink, Linkname: "/outside"}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolutePath, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTar(absolutePath, filepath.Join(root, "absolute-extract")); err == nil {
		t.Fatal("absolute symlink accepted")
	}
	safePath := filepath.Join(root, "safe.tar.gz")
	data.Reset()
	gz = gzip.NewWriter(&data)
	tw = tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "inside", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "inside/link", Typeflag: tar.TypeSymlink, Linkname: "../inside"}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(safePath, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	safeRoot := filepath.Join(root, "safe-extract")
	if err := os.Mkdir(safeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractTar(safePath, safeRoot); err != nil {
		t.Fatalf("safe nested symlink rejected: %v", err)
	}
	corrupt := filepath.Join(root, "corrupt.tar.gz")
	if err := os.WriteFile(corrupt, []byte("not a tar"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTar(corrupt, filepath.Join(root, "corrupt-extract")); err == nil {
		t.Fatal("corrupt tar accepted")
	}
}
