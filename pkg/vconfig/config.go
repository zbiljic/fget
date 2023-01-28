package vconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/fatih/structs"
)

// CheckData checks the validity of config data. Data should be of
// type struct and contain a string type field called "Version".
func CheckData(data any) error {
	if !structs.IsStruct(data) {
		return fmt.Errorf("interface must be struct type")
	}

	st := structs.New(data)
	f, ok := st.FieldOk("Version")
	if !ok {
		return fmt.Errorf("struct '%s' must have field 'Version'", st.Name())
	}

	if f.Kind() != reflect.String {
		return fmt.Errorf("'Version' field in struct '%s' must be a string type", st.Name())
	}

	return nil
}

// GetVersion extracts the version information from file.
func GetVersion(filename string) (string, error) {
	config, err := LoadConfig[struct{ Version string }](filename)
	if err != nil {
		return "", err
	}

	return config.Version, nil
}

// LoadConfig loads JSON config from filename.
func LoadConfig[T any](filename string) (*T, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	if runtime.GOOS == "windows" {
		data = []byte(strings.ReplaceAll(string(data), "\r\n", "\n"))
	}

	config, err := jsonUnmarshalAny[T](data)
	if err != nil {
		return nil, err
	}

	if err := CheckData(config); err != nil {
		return nil, err
	}

	return config, nil
}

// SaveConfig saves given configuration data into given file as JSON.
func SaveConfig(config any, filename string) error {
	if err := CheckData(config); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "\t")
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		data = []byte(strings.ReplaceAll(string(data), "\n", "\r\n"))
	}

	return os.WriteFile(filename, data, 0o644)
}
