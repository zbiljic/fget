package vconfig

import "encoding/json"

func jsonUnmarshalAny[T any](data []byte) (*T, error) {
	out := new(T)
	if err := json.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}
