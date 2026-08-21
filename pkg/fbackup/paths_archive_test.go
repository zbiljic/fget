package fbackup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateRejectsInvalidManifestAndClassification(t *testing.T) {
	cases := []struct {
		name           string
		version        string
		classification Classification
	}{
		{"missing classification", ManifestVersion, ""},
		{"problem", ManifestVersion, ClassificationProblem},
		{"unknown", ManifestVersion, ClassificationUnknown},
		{"unsupported", ManifestVersion, Classification("future")},
		{"version", "2", ClassificationRecloneable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Create(context.Background(), CreateOptions{Destination: filepath.Join(t.TempDir(), "backup"), Manifest: Manifest{Version: tc.version, Repositories: []RepositoryEntry{{ID: "id", Classification: tc.classification}}}})
			if err == nil {
				t.Fatal("Create() succeeded")
			}
		})
	}
}

func TestCreateRejectsDestinationOverlapAndSymlinkedArtifactDirectory(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "source"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Create(context.Background(), CreateOptions{Destination: filepath.Join(repo, "backup"), Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{ID: "id", Path: repo, Classification: ClassificationFull}}}}); err == nil {
		t.Fatal("Create() allowed destination inside source repository")
	}
	alias := filepath.Join(root, "repo-alias")
	if err := os.Symlink(repo, alias); err != nil {
		t.Fatal(err)
	}
	if err := Create(context.Background(), CreateOptions{Destination: filepath.Join(alias, "backup"), Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{{ID: "alias", Path: alias, Classification: ClassificationFull}}}}); err == nil {
		t.Fatal("Create() allowed destination inside symlinked source repository")
	}
}

func TestRepositoryDirNameSafeForArbitraryIDs(t *testing.T) {
	for _, id := range []string{"a/b", "host:repo", "a%2Fb", "日本語", ".", ".."} {
		name := repositoryDirName(id)
		if !safeRelativePath(filepath.ToSlash(filepath.Join("repos", name, "full.tar.gz"))) {
			t.Fatalf("repositoryDirName(%q) = %q is unsafe", id, name)
		}
	}
}

func TestWriteTarRejectsTraversalAndDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "archive.tar.gz")
	seed, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()
	if err := writeTar(context.Background(), root, out, []string{"../escape"}); err == nil {
		t.Fatal("writeTar() accepted traversal path")
	}
	if err := writeTar(context.Background(), root, out, []string{"link", "inside"}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	seenLink := false
	var names []string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "link" {
			seenLink = h.Typeflag == tar.TypeSymlink
		}
		names = append(names, h.Name)
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "secret-content") {
			t.Fatal("symlink target contents entered archive")
		}
	}
	_ = gz.Close()
	_ = f.Close()
	if !seenLink {
		t.Fatal("symlink was not recorded")
	}
	if strings.Join(names, ",") != "inside,link" {
		t.Fatalf("tar member order = %v, want [inside link]", names)
	}
}
