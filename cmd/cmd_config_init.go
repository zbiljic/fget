package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fconfig"
	"github.com/zbiljic/fget/pkg/fsfind"
	"github.com/zbiljic/fget/pkg/vconfig"
)

const fconfigFilename = "fget.yaml"

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create or update fget.yaml configuration",
	Args:  cobra.NoArgs,
	RunE:  runConfigInit,
}

type configInitOptions struct {
	Roots   []string
	Force   bool
	Local   bool
	File    string
	Catalog bool
}

var configInitCmdFlags = configInitOptions{}

var errConfigInitConflictingTargetFlags = errors.New("cannot use --local and --file together")

func init() {
	configInitCmd.Flags().StringArrayVar(&configInitCmdFlags.Roots, "root", nil, "Root directories to include in config")
	configInitCmd.Flags().BoolVar(&configInitCmdFlags.Force, "force", false, "Overwrite existing configuration with minimal config")
	configInitCmd.Flags().BoolVar(&configInitCmdFlags.Local, "local", false, "Write config to ./fget.yaml")
	configInitCmd.Flags().StringVar(&configInitCmdFlags.File, "file", "", "Explicit config file path")
	configInitCmd.Flags().BoolVar(&configInitCmdFlags.Catalog, "catalog", false, "Set catalog.path to a sibling ./fget.catalog.yaml")

	configCmd.AddCommand(configInitCmd)
}

func runConfigInit(_ *cobra.Command, _ []string) error {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return err
	}

	targetPath, err := resolveInitTargetPath(
		configInitCmdFlags.Local,
		configInitCmdFlags.File,
		runtimeCtx.Cwd,
		runtimeCtx.HomeDir,
		runtimeCtx.XDGConfigHome,
	)
	if err != nil {
		return err
	}

	roots, err := resolveInitRoots(configInitCmdFlags.Roots, runtimeCtx.Cwd, runtimeCtx.HomeDir)
	if err != nil {
		return err
	}

	config, err := applyInitConfig(targetPath, roots, configInitCmdFlags.Force, configInitCmdFlags.Catalog)
	if err != nil {
		return err
	}

	ptermSuccessMessageStyle.Printfln("config initialized: %s", targetPath)
	pterm.Printf("roots: %s\n", strings.Join(config.Roots, ", "))
	if config.Catalog.Path != "" {
		pterm.Printf("catalog: %s\n", config.Catalog.Path)
	}
	pterm.Println("next: fget catalog sync")

	return nil
}

func resolveInitTargetPath(local bool, file, cwd, homeDir, xdgConfigHome string) (string, error) {
	if local && file != "" {
		return "", errConfigInitConflictingTargetFlags
	}

	if file != "" {
		file = expandHomePath(file, homeDir)
		if filepath.IsAbs(file) {
			return filepath.Clean(file), nil
		}

		return filepath.Join(cwd, file), nil
	}

	if local {
		return filepath.Join(cwd, fconfigFilename), nil
	}

	return fconfig.ResolveBaseConfigPath(xdgConfigHome, homeDir), nil
}

func resolveInitRoots(flagRoots []string, cwd, homeDir string) ([]string, error) {
	roots := flagRoots
	if len(roots) == 0 {
		roots = []string{cwd}
	}

	normalizedRoots := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))

	for _, root := range roots {
		root = expandHomePath(root, homeDir)

		path, err := fsfind.DirAbsPath(root)
		if err != nil {
			return nil, fmt.Errorf("invalid root %q: %w", root, err)
		}

		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		normalizedRoots = append(normalizedRoots, path)
	}

	sort.Strings(normalizedRoots)
	return normalizedRoots, nil
}

func applyInitConfig(target string, roots []string, force, withCatalog bool) (*fconfig.Config, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}

	roots = sortedUnique(roots)

	config := &fconfig.Config{
		Version: fconfig.ConfigVersionV2,
		Roots:   roots,
	}

	if !force && fileExists(target) {
		existing, err := vconfig.LoadConfig[fconfig.Config](target)
		if err != nil {
			return nil, err
		}

		if existing.Version != "" {
			config.Version = existing.Version
		}
		config.Catalog = existing.Catalog
		config.Roots = sortedUnique(append(existing.Roots, roots...))
	}

	if withCatalog {
		config.Version = fconfig.ConfigVersionV2
		config.Catalog.Path = fconfig.ResolveScopedCatalogPath()
	}

	if err := vconfig.SaveConfig(config, target); err != nil {
		return nil, err
	}

	return config, nil
}

func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))

	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	sort.Strings(out)
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}
