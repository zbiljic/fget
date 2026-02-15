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

type TagsConfig struct {
	Defaults []string `yaml:"defaults" json:"defaults"`
}

type Config struct {
	Version string        `yaml:"version" json:"version"`
	Roots   []string      `yaml:"roots" json:"roots"`
	Catalog CatalogConfig `yaml:"catalog" json:"catalog"`
	Tags    TagsConfig    `yaml:"tags" json:"tags"`
}
