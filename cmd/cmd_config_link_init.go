package cmd

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fconfig"
	"github.com/zbiljic/fget/pkg/vconfig"
)

var configLinkInitCmd = &cobra.Command{
	Use:   "init <tag...>",
	Short: "Create or update local link projection config",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runConfigLinkInit,
}

type configLinkInitOptions struct {
	Root       string
	SourceRoot string
	Match      string
	Layout     string
}

var configLinkInitCmdFlags = configLinkInitOptions{}

func init() {
	configLinkInitCmd.Flags().StringVar(&configLinkInitCmdFlags.Root, "root", "", "Managed symlink root relative to the config file")
	configLinkInitCmd.Flags().StringVar(
		&configLinkInitCmdFlags.SourceRoot,
		"source-root",
		"",
		"Preferred source root when repositories have multiple catalog locations",
	)
	configLinkInitCmd.Flags().StringVar(&configLinkInitCmdFlags.Match, "match", "", "Tag match mode: any or all")
	configLinkInitCmd.Flags().StringVar(&configLinkInitCmdFlags.Layout, "layout", "", "Link layout: repo-id")

	configLinkCmd.AddCommand(configLinkInitCmd)
}

func runConfigLinkInit(_ *cobra.Command, args []string) error {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return err
	}

	targetPath := resolveInitLinkTargetPath(runtimeCtx.Cwd)

	var existingLink *fconfig.LinkConfig
	if fileExists(targetPath) {
		existingConfig, err := vconfig.LoadConfig[fconfig.Config](targetPath)
		if err != nil {
			return err
		}
		existingLink = existingConfig.Link
	}

	linkConfig, err := buildLinkInitConfig(existingLink, args, configLinkInitCmdFlags)
	if err != nil {
		return err
	}

	if _, err := applyLinkInitConfig(targetPath, *linkConfig); err != nil {
		return err
	}

	ptermSuccessMessageStyle.Printfln("link config initialized: %s", targetPath)
	return nil
}

func resolveInitLinkTargetPath(cwd string) string {
	return filepath.Join(cwd, fconfigFilename)
}

func buildLinkInitConfig(
	existing *fconfig.LinkConfig,
	tags []string,
	opts configLinkInitOptions,
) (*fconfig.LinkConfig, error) {
	normalizedTags := normalizeConfigTags(tags)
	if len(normalizedTags) == 0 {
		return nil, errors.New("requires at least one link tag")
	}

	match := strings.TrimSpace(opts.Match)
	if match == "" && existing != nil {
		match = existing.Match
	}
	if match == "" {
		match = fconfig.LinkMatchAny
	}
	switch match {
	case fconfig.LinkMatchAny, fconfig.LinkMatchAll:
	default:
		return nil, errors.New("invalid link match mode")
	}

	layout := strings.TrimSpace(opts.Layout)
	if layout == "" && existing != nil {
		layout = existing.Layout
	}
	if layout == "" {
		layout = fconfig.LinkLayoutRepoID
	}
	switch layout {
	case fconfig.LinkLayoutRepoID:
	default:
		return nil, errors.New("invalid link layout")
	}

	root := strings.TrimSpace(opts.Root)
	if root == "" && existing != nil {
		root = strings.TrimSpace(existing.Root)
	}
	if root == "" {
		root = "."
	}

	sourceRoot := strings.TrimSpace(opts.SourceRoot)
	if sourceRoot == "" && existing != nil {
		sourceRoot = strings.TrimSpace(existing.SourceRoot)
	}

	return &fconfig.LinkConfig{
		Tags:       normalizedTags,
		Match:      match,
		Layout:     layout,
		Root:       root,
		SourceRoot: sourceRoot,
	}, nil
}

func applyLinkInitConfig(target string, linkConfig fconfig.LinkConfig) (*fconfig.Config, error) {
	config := &fconfig.Config{
		Version: fconfig.ConfigVersionV2,
		Link:    &linkConfig,
	}

	if fileExists(target) {
		existing, err := vconfig.LoadConfig[fconfig.Config](target)
		if err != nil {
			return nil, err
		}

		if existing.Version != "" {
			config.Version = existing.Version
		}
		config.Roots = existing.Roots
		config.Catalog = existing.Catalog
	}

	if err := vconfig.SaveConfig(config, target); err != nil {
		return nil, err
	}

	return config, nil
}

func normalizeConfigTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}

		seen[tag] = struct{}{}
		out = append(out, tag)
	}

	sort.Strings(out)
	return out
}
