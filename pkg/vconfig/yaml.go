package vconfig

import "gopkg.in/yaml.v3"

func yamlUnmarshalAny[T any](data []byte) (*T, error) {
	out := new(T)
	if err := yaml.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func yamlMarshal(config any) ([]byte, error) {
	return yaml.Marshal(config)
}
