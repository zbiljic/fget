package cmd

import (
	"cmp"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fconfig"
	"github.com/zbiljic/fget/pkg/giturl"
)

const catalogExportSchemaVersion = "1"

type catalogExportFlags struct {
	CatalogPath   string
	ScopeRoot     string
	Output        string
	OutputFile    string
	LocationRoots []string
	Hosts         []string
	Tags          []string
	Sort          string
	BatchSize     int
	Batch         int
}

type catalogExportRecord struct {
	SchemaVersion string    `json:"schema_version"`
	CatalogDigest string    `json:"catalog_digest"`
	Ordinal       int       `json:"ordinal"`
	Batch         int       `json:"batch"`
	ID            string    `json:"id"`
	RemoteURL     string    `json:"remote_url"`
	Location      string    `json:"location"`
	Host          string    `json:"host"`
	Owner         string    `json:"owner"`
	Tags          []string  `json:"tags"`
	LastSeenAt    time.Time `json:"last_seen_at"`
}

var catalogExportCmdFlags catalogExportFlags

var catalogExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export deterministic catalog location records",
	Args:  cobra.NoArgs,
	RunE:  runCatalogExport,
}

func init() {
	catalogExportCmd.Flags().StringVar(&catalogExportCmdFlags.CatalogPath, "catalog", "", "Explicit catalog file")
	catalogExportCmd.Flags().StringVar(&catalogExportCmdFlags.ScopeRoot, "scope-root", "", "Root used to resolve relative catalog locations")
	catalogExportCmd.Flags().StringVarP(&catalogExportCmdFlags.Output, "output", "o", "jsonl", "Output format: json, jsonl, or tsv")
	catalogExportCmd.Flags().StringVar(&catalogExportCmdFlags.OutputFile, "output-file", "-", "Output file, or - for stdout")
	catalogExportCmd.Flags().StringSliceVar(&catalogExportCmdFlags.LocationRoots, "location-root", nil, "Include locations under this root (repeatable)")
	catalogExportCmd.Flags().StringSliceVar(&catalogExportCmdFlags.Hosts, "host", nil, "Include repository host (repeatable)")
	catalogExportCmd.Flags().StringSliceVar(&catalogExportCmdFlags.Tags, "tag", nil, "Include repositories with any tag (repeatable)")
	catalogExportCmd.Flags().StringVar(&catalogExportCmdFlags.Sort, "sort", "id", "Sort by id, host, or path")
	catalogExportCmd.Flags().IntVar(&catalogExportCmdFlags.BatchSize, "batch-size", 0, "Assign deterministic batches of at most N records")
	catalogExportCmd.Flags().IntVar(&catalogExportCmdFlags.Batch, "batch", 0, "Emit only this 1-based batch")
}

func runCatalogExport(cmd *cobra.Command, _ []string) error {
	if err := validateCatalogExportFlags(catalogExportCmdFlags); err != nil {
		return err
	}

	catalog, digest, err := loadCatalogForExport(catalogExportCmdFlags)
	if err != nil {
		return err
	}

	records, err := buildCatalogExportRecords(catalog, digest, catalogExportCmdFlags)
	if err != nil {
		return err
	}

	write := func(w io.Writer) error {
		return writeCatalogExport(w, catalogExportCmdFlags.Output, records)
	}
	if catalogExportCmdFlags.OutputFile == "-" {
		return write(cmd.OutOrStdout())
	}

	return writeCatalogExportFile(catalogExportCmdFlags.OutputFile, write)
}

func validateCatalogExportFlags(flags catalogExportFlags) error {
	switch flags.Output {
	case "json", "jsonl", "tsv":
	default:
		return fmt.Errorf("unsupported output format %q", flags.Output)
	}

	switch flags.Sort {
	case "id", "host", "path":
	default:
		return fmt.Errorf("unsupported sort %q", flags.Sort)
	}

	if flags.ScopeRoot != "" && flags.CatalogPath == "" {
		return errors.New("--scope-root requires --catalog")
	}
	if flags.BatchSize < 0 {
		return errors.New("--batch-size must not be negative")
	}
	if flags.Batch < 0 {
		return errors.New("--batch must not be negative")
	}
	if flags.Batch > 0 && flags.BatchSize == 0 {
		return errors.New("--batch requires --batch-size")
	}

	return nil
}

func loadCatalogForExport(flags catalogExportFlags) (*fconfig.Catalog, string, error) {
	if flags.CatalogPath == "" {
		set, err := loadCatalogSetForCurrentRuntimeContext()
		if err != nil {
			return nil, "", err
		}
		digest, err := catalogExportDigest(set.View)
		return set.View, digest, err
	}

	catalogPath, err := filepath.Abs(flags.CatalogPath)
	if err != nil {
		return nil, "", err
	}
	catalogBytes, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(catalogBytes)
	scopeRoot := flags.ScopeRoot
	if scopeRoot == "" {
		scopeRoot = filepath.Dir(catalogPath)
	} else {
		scopeRoot, err = filepath.Abs(scopeRoot)
		if err != nil {
			return nil, "", err
		}
	}

	catalog, err := fconfig.LoadCatalogData(catalogBytes, catalogPath, scopeRoot)
	if err != nil {
		return nil, "", err
	}
	return catalog, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func buildCatalogExportRecords(
	catalog *fconfig.Catalog,
	digest string,
	flags catalogExportFlags,
) ([]catalogExportRecord, error) {
	if catalog == nil {
		return nil, errors.New("nil catalog")
	}

	if digest == "" {
		var err error
		digest, err = catalogExportDigest(catalog)
		if err != nil {
			return nil, err
		}
	}

	hosts := normalizedStringSet(flags.Hosts)
	tags := normalizedStringSet(flags.Tags)
	locationRoots, err := absoluteCleanPaths(flags.LocationRoots)
	if err != nil {
		return nil, fmt.Errorf("resolve location roots: %w", err)
	}
	records := make([]catalogExportRecord, 0, len(catalog.Repos))

	for _, repo := range catalog.Repos {
		host, owner := splitCatalogRepoID(repo.ID)
		if len(hosts) > 0 {
			if _, ok := hosts[strings.ToLower(host)]; !ok {
				continue
			}
		}
		if len(tags) > 0 && !hasAnyCatalogTag(repo.Tags, tags) {
			continue
		}

		repoTags := append([]string{}, repo.Tags...)
		sort.Strings(repoTags)
		for _, location := range repo.Locations {
			if len(locationRoots) > 0 && !pathUnderAnyRoot(location.Path, locationRoots) {
				continue
			}
			records = append(records, catalogExportRecord{
				SchemaVersion: catalogExportSchemaVersion,
				CatalogDigest: digest,
				ID:            repo.ID,
				RemoteURL:     giturl.Sanitize(repo.RemoteURL),
				Location:      filepath.Clean(location.Path),
				Host:          host,
				Owner:         owner,
				Tags:          repoTags,
				LastSeenAt:    location.LastSeenAt,
			})
		}
	}

	sortCatalogExportRecords(records, flags.Sort)
	selected := records[:0]
	for i := range records {
		records[i].Ordinal = i + 1
		records[i].Batch = 1
		if flags.BatchSize > 0 {
			records[i].Batch = i/flags.BatchSize + 1
		}
		if flags.Batch == 0 || records[i].Batch == flags.Batch {
			selected = append(selected, records[i])
		}
	}

	return selected, nil
}

func catalogExportDigest(catalog *fconfig.Catalog) (string, error) {
	encoded, err := json.Marshal(catalog)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func normalizedStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func absoluteCleanPaths(paths []string) ([]string, error) {
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}

		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		cleaned = append(cleaned, filepath.Clean(absolute))
	}
	return cleaned, nil
}

func splitCatalogRepoID(id string) (string, string) {
	parts := strings.Split(strings.Trim(id, "/"), "/")
	if len(parts) == 0 {
		return "", ""
	}
	host := parts[0]
	owner := ""
	if len(parts) > 1 {
		owner = parts[1]
	}
	return host, owner
}

func hasAnyCatalogTag(repoTags []string, wanted map[string]struct{}) bool {
	for _, tag := range repoTags {
		if _, ok := wanted[strings.ToLower(tag)]; ok {
			return true
		}
	}
	return false
}

func pathUnderAnyRoot(path string, roots []string) bool {
	path = filepath.Clean(path)
	for _, root := range roots {
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func sortCatalogExportRecords(records []catalogExportRecord, order string) {
	sort.SliceStable(records, func(i, j int) bool {
		a, b := records[i], records[j]
		switch order {
		case "host":
			return cmp.Or(
				cmp.Compare(a.Host, b.Host),
				cmp.Compare(a.Owner, b.Owner),
				cmp.Compare(a.ID, b.ID),
				cmp.Compare(a.Location, b.Location),
			) < 0
		case "path":
			return cmp.Or(
				cmp.Compare(a.Location, b.Location),
				cmp.Compare(a.ID, b.ID),
			) < 0
		default:
			return cmp.Or(
				cmp.Compare(a.ID, b.ID),
				cmp.Compare(a.Location, b.Location),
			) < 0
		}
	})
}

func writeCatalogExport(w io.Writer, format string, records []catalogExportRecord) error {
	switch format {
	case "json":
		return outputJSON(w, records)
	case "jsonl":
		encoder := json.NewEncoder(w)
		for _, record := range records {
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	case "tsv":
		return writeCatalogExportTSV(w, records)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func writeCatalogExportTSV(w io.Writer, records []catalogExportRecord) error {
	writer := csv.NewWriter(w)
	writer.Comma = '\t'
	if err := writer.Write([]string{
		"schema_version", "catalog_digest", "ordinal", "batch", "id", "remote_url",
		"location", "host", "owner", "tags", "last_seen_at",
	}); err != nil {
		return err
	}
	for _, record := range records {
		if err := writer.Write([]string{
			record.SchemaVersion,
			record.CatalogDigest,
			strconv.Itoa(record.Ordinal),
			strconv.Itoa(record.Batch),
			record.ID,
			record.RemoteURL,
			record.Location,
			record.Host,
			record.Owner,
			strings.Join(record.Tags, ","),
			record.LastSeenAt.Format(time.RFC3339Nano),
		}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func writeCatalogExportFile(path string, write func(io.Writer) error) (err error) {
	return writeAtomicOutputFile(path, ".fget-catalog-export-*", write)
}
