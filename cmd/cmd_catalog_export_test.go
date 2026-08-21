package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/zbiljic/fget/pkg/fconfig"
)

func TestBuildCatalogExportRecordsFiltersSortsAndBatches(t *testing.T) {
	t.Parallel()

	seenAt := time.Date(2026, time.August, 13, 21, 45, 16, 0, time.UTC)
	catalog := &fconfig.Catalog{
		Version:   fconfig.CatalogVersionV1,
		UpdatedAt: seenAt,
		Repos: []fconfig.RepoEntry{
			{
				ID:        "github.com/zeta/one",
				RemoteURL: "https://github.com/zeta/one",
				Tags:      []string{"priority"},
				Locations: []fconfig.RepoLocation{
					{Path: "/source/src/github.com/zeta/one", LastSeenAt: seenAt},
					{Path: "/source/q__manual__src___/github.com/zeta/one", LastSeenAt: seenAt},
				},
			},
			{
				ID:        "github.com/alpha/two",
				RemoteURL: "https://github.com/alpha/two",
				Tags:      []string{"priority", "work"},
				Locations: []fconfig.RepoLocation{
					{Path: "/source/src/github.com/alpha/two", LastSeenAt: seenAt},
				},
			},
			{
				ID:        "gitlab.example.com/acme/three",
				RemoteURL: "https://gitlab.example.com/acme/three",
				Tags:      []string{"priority"},
				Locations: []fconfig.RepoLocation{
					{Path: "/source/src/gitlab.example.com/acme/three", LastSeenAt: seenAt},
				},
			},
		},
	}

	flags := catalogExportFlags{
		LocationRoots: []string{"/source/src"},
		Hosts:         []string{"GITHUB.COM"},
		Tags:          []string{"priority"},
		Sort:          "host",
		BatchSize:     1,
		Batch:         2,
	}
	records, err := buildCatalogExportRecords(catalog, "sha256:snapshot", flags)
	if err != nil {
		t.Fatalf("buildCatalogExportRecords() error = %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	record := records[0]
	if record.ID != "github.com/zeta/one" || record.Location != "/source/src/github.com/zeta/one" {
		t.Fatalf("record = %+v", record)
	}
	if record.Ordinal != 2 || record.Batch != 2 {
		t.Fatalf("ordinal/batch = %d/%d, want 2/2", record.Ordinal, record.Batch)
	}
	if record.Host != "github.com" || record.Owner != "zeta" {
		t.Fatalf("host/owner = %q/%q", record.Host, record.Owner)
	}
	if record.CatalogDigest != "sha256:snapshot" {
		t.Fatalf("catalog digest = %q, want supplied snapshot digest", record.CatalogDigest)
	}
}

func TestBuildCatalogExportRecordsIsDeterministic(t *testing.T) {
	t.Parallel()

	catalog := &fconfig.Catalog{
		Version: fconfig.CatalogVersionV1,
		Repos: []fconfig.RepoEntry{
			{ID: "github.com/b/repo", Tags: []string{"z", "a"}, Locations: []fconfig.RepoLocation{{Path: "/src/b"}}},
			{ID: "github.com/a/repo", Locations: []fconfig.RepoLocation{{Path: "/src/a"}}},
		},
	}
	flags := catalogExportFlags{Sort: "path", BatchSize: 100}

	first, err := buildCatalogExportRecords(catalog, "", flags)
	if err != nil {
		t.Fatalf("first buildCatalogExportRecords() error = %v", err)
	}
	second, err := buildCatalogExportRecords(catalog, "", flags)
	if err != nil {
		t.Fatalf("second buildCatalogExportRecords() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("records differ:\nfirst:  %+v\nsecond: %+v", first, second)
	}
	if got := first[1].Tags; !reflect.DeepEqual(got, []string{"a", "z"}) {
		t.Fatalf("sorted tags = %v, want [a z]", got)
	}
	if first[0].Tags == nil {
		t.Fatal("empty tags must encode as an empty array")
	}
	if got := catalog.Repos[0].Tags; !reflect.DeepEqual(got, []string{"z", "a"}) {
		t.Fatalf("source catalog tags mutated to %v", got)
	}
}

func TestBuildCatalogExportRecordsSanitizesRemoteURL(t *testing.T) {
	t.Parallel()

	catalog := &fconfig.Catalog{
		Version: fconfig.CatalogVersionV1,
		Repos: []fconfig.RepoEntry{{
			ID:        "example.com/acme/repo",
			RemoteURL: "https://user:secret@example.com/acme/repo.git",
			Locations: []fconfig.RepoLocation{{Path: "/src/repo"}},
		}},
	}

	records, err := buildCatalogExportRecords(catalog, "sha256:snapshot", catalogExportFlags{Sort: "id"})
	if err != nil {
		t.Fatalf("buildCatalogExportRecords() error = %v", err)
	}
	if len(records) != 1 || records[0].RemoteURL != "https://example.com/acme/repo.git" {
		t.Fatalf("records = %+v, want sanitized remote URL", records)
	}
}

func TestBuildCatalogExportRecordsResolvesRelativeLocationRoots(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	catalog := &fconfig.Catalog{
		Version: fconfig.CatalogVersionV1,
		Repos: []fconfig.RepoEntry{{
			ID: "github.com/acme/repo",
			Locations: []fconfig.RepoLocation{
				{Path: filepath.Join(cwd, "relative-root", "repo")},
			},
		}},
	}

	records, err := buildCatalogExportRecords(catalog, "", catalogExportFlags{
		LocationRoots: []string{"relative-root"},
		Sort:          "id",
	})
	if err != nil {
		t.Fatalf("buildCatalogExportRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
}

func TestWriteCatalogExportJSONLAndTSV(t *testing.T) {
	t.Parallel()

	record := catalogExportRecord{
		SchemaVersion: "1",
		CatalogDigest: "sha256:abc",
		Ordinal:       3,
		Batch:         2,
		ID:            "github.com/acme/repo",
		RemoteURL:     "https://github.com/acme/repo",
		Location:      "/source/src/github.com/acme/repo",
		Host:          "github.com",
		Owner:         "acme",
		Tags:          []string{"one", "two"},
		LastSeenAt:    time.Date(2026, time.August, 13, 21, 45, 16, 123, time.UTC),
	}

	var jsonl bytes.Buffer
	if err := writeCatalogExport(&jsonl, "jsonl", []catalogExportRecord{record}); err != nil {
		t.Fatalf("writeCatalogExport(jsonl) error = %v", err)
	}
	var decoded catalogExportRecord
	if err := json.Unmarshal(jsonl.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, record) {
		t.Fatalf("decoded = %+v, want %+v", decoded, record)
	}

	var tsv bytes.Buffer
	if err := writeCatalogExport(&tsv, "tsv", []catalogExportRecord{record}); err != nil {
		t.Fatalf("writeCatalogExport(tsv) error = %v", err)
	}
	reader := csv.NewReader(&tsv)
	reader.Comma = '\t'
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(rows) != 2 || rows[1][4] != record.ID || rows[1][9] != "one,two" {
		t.Fatalf("TSV rows = %v", rows)
	}
}

func TestLoadCatalogForExportUsesFileDigestAndExplicitScope(t *testing.T) {
	t.Parallel()

	catalogData := []byte("version: \"1\"\nupdated_at: 2026-08-13T21:45:16Z\nroots:\n  - path: src\n    last_scanned_at: 2026-08-13T21:45:16Z\nrepos:\n  - id: github.com/acme/repo\n    remote_url: https://github.com/acme/repo\n    tags: []\n    locations:\n      - path: src/github.com/acme/repo\n        last_seen_at: 2026-08-13T21:45:16Z\n")
	catalogPath := filepath.Join(t.TempDir(), "snapshot.yaml")
	if err := os.WriteFile(catalogPath, catalogData, 0o644); err != nil {
		t.Fatal(err)
	}
	scopeRoot := filepath.Join(t.TempDir(), "source")

	catalog, digest, err := loadCatalogForExport(catalogExportFlags{
		CatalogPath: catalogPath,
		ScopeRoot:   scopeRoot,
	})
	if err != nil {
		t.Fatalf("loadCatalogForExport() error = %v", err)
	}
	wantSum := sha256.Sum256(catalogData)
	wantDigest := "sha256:" + hex.EncodeToString(wantSum[:])
	if digest != wantDigest {
		t.Fatalf("digest = %q, want %q", digest, wantDigest)
	}
	wantLocation := filepath.Join(scopeRoot, "src/github.com/acme/repo")
	if got := catalog.Repos[0].Locations[0].Path; got != wantLocation {
		t.Fatalf("location = %q, want %q", got, wantLocation)
	}
}

func TestWriteCatalogExportFileReplacesAtomically(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "inventory.jsonl")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeCatalogExportFile(path, func(w io.Writer) error {
		_, err := io.WriteString(w, "new\n")
		return err
	}); err != nil {
		t.Fatalf("writeCatalogExportFile() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Fatalf("file = %q, want new", got)
	}
}

func TestWriteCatalogExportFilePreservesDestinationOnWriteFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.jsonl")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("write failed")
	if err := writeCatalogExportFile(path, func(w io.Writer) error {
		if _, err := io.WriteString(w, "partial\n"); err != nil {
			return err
		}
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("writeCatalogExportFile() error = %v, want %v", err, wantErr)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old\n" {
		t.Fatalf("file = %q, want original contents", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "inventory.jsonl" {
		t.Fatalf("directory entries = %v, want only destination", entries)
	}
}

func TestValidateCatalogExportFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		flags catalogExportFlags
	}{
		{name: "bad format", flags: catalogExportFlags{Output: "yaml", Sort: "id"}},
		{name: "bad sort", flags: catalogExportFlags{Output: "jsonl", Sort: "time"}},
		{name: "scope without catalog", flags: catalogExportFlags{Output: "jsonl", Sort: "id", ScopeRoot: "/source"}},
		{name: "negative batch size", flags: catalogExportFlags{Output: "jsonl", Sort: "id", BatchSize: -1}},
		{name: "negative batch", flags: catalogExportFlags{Output: "jsonl", Sort: "id", Batch: -1}},
		{name: "batch without size", flags: catalogExportFlags{Output: "jsonl", Sort: "id", Batch: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validateCatalogExportFlags(test.flags); err == nil {
				t.Fatal("validateCatalogExportFlags() error = nil")
			}
		})
	}
}
