package cmd

import (
	"encoding/json"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fconfig"
)

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show effective merged config",
	Args:  cobra.NoArgs,
	RunE:  runConfigShow,
}

func init() {
	configCmd.AddCommand(configShowCmd)
}

type configShowOutput struct {
	Version string                `json:"version"`
	Roots   []string              `json:"roots"`
	Catalog fconfig.CatalogConfig `json:"catalog"`
	Link    *fconfig.LinkConfig   `json:"link,omitempty"`
	Sources []string              `json:"sources"`
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	ctx, err := loadConfigRuntimeContext()
	if err != nil {
		return err
	}

	config, err := fconfig.LoadEffectiveConfig(ctx.HomeDir, ctx.Cwd, ctx.XDGConfigHome)
	if err != nil {
		return err
	}

	out := configShowOutput{
		Version: config.Version,
		Roots:   config.Roots,
		Catalog: config.Catalog,
		Link:    config.Link,
		Sources: config.Sources,
	}

	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}

	pterm.Println(string(enc))
	return nil
}
