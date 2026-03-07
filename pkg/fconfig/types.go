package fconfig

const (
	ConfigVersionV1  = "1"
	configFilename   = "fget.yaml"
	catalogFilename  = "catalog.yaml"
	configDirname    = "fget"
	defaultConfigDir = ".config"
)

type CatalogConfig struct {
	Path string `yaml:"path" json:"path"`
}

type LinkConfig struct {
	Tags       []string `yaml:"tags" json:"tags"`
	Match      string   `yaml:"match" json:"match"`
	Layout     string   `yaml:"layout" json:"layout"`
	Root       string   `yaml:"root" json:"root"`
	SourceRoot string   `yaml:"source_root" json:"source_root"`
}

type Config struct {
	Version string        `yaml:"version" json:"version"`
	Roots   []string      `yaml:"roots" json:"roots"`
	Catalog CatalogConfig `yaml:"catalog" json:"catalog"`
	Link    *LinkConfig   `yaml:"link,omitempty" json:"link,omitempty"`
}
