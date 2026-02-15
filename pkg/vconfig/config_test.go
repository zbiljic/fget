package vconfig

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestReadVersion(t *testing.T) {
	type myStruct struct {
		Version string
	}
	saveMe := myStruct{"1"}
	err := SaveConfig(&saveMe, "test.json")
	if err != nil {
		t.Fatal(err)
	}

	version, err := GetVersion("test.json")
	if err != nil {
		t.Fatal(err)
	}
	if version != "1" {
		t.Fatalf("Expected version '1', got '%v'", version)
	}
}

func TestReadVersionErr(t *testing.T) {
	err := os.WriteFile("test.json", []byte("{ \"version\":2,"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = GetVersion("test.json")
	if err == nil {
		t.Fatal("Unexpected should fail to fetch version")
	}

	defer os.Remove("test.json") //nolint:errcheck
	err = os.WriteFile("test.json", []byte("{ \"version\":2 }"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = GetVersion("test.json")
	if err == nil {
		t.Fatal("Unexpected should fail to fetch version")
	}
}

func TestSaveFailOnDir(t *testing.T) {
	defer os.RemoveAll("test-1.json") //nolint:errcheck
	err := os.MkdirAll("test-1.json", 0o644)
	if err != nil {
		t.Fatal(err)
	}

	type myStruct struct {
		Version string
	}
	saveMe := myStruct{"1"}
	err = SaveConfig(&saveMe, "test-1.json")
	if err == nil {
		t.Fatal("Unexpected should fail to save if test-1.json is a directory")
	}
}

func TestCheckData(t *testing.T) {
	err := CheckData(nil)
	if err == nil {
		t.Fatal("Unexpected should fail")
	}

	type myStructBadNoVersion struct {
		User        string
		Password    string
		Directories []string
	}
	saveMeBadNoVersion := myStructBadNoVersion{"guest", "nopassword", []string{"Work", "Documents", "Music"}}
	err = CheckData(&saveMeBadNoVersion)
	if err == nil {
		t.Fatal("Unexpected should fail if Version is not set")
	}

	type myStructBadVersionInt struct {
		Version  int
		User     string
		Password string
	}
	saveMeBadVersionInt := myStructBadVersionInt{1, "guest", "nopassword"}
	err = CheckData(&saveMeBadVersionInt)
	if err == nil {
		t.Fatal("Unexpected should fail if Version is integer")
	}

	type myStructGood struct {
		Version     string
		User        string
		Password    string
		Directories []string
	}

	saveMeGood := myStructGood{"1", "guest", "nopassword", []string{"Work", "Documents", "Music"}}
	err = CheckData(&saveMeGood)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadFile(t *testing.T) {
	type myStruct struct {
		Version     string
		User        string
		Password    string
		Directories []string
	}
	saveMe := myStruct{}
	_, err := LoadConfig[myStruct]("test.json")
	if err == nil {
		t.Fatal(err)
	}

	file, err := os.Create("test.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = LoadConfig[myStruct]("test.json")
	if err == nil {
		t.Fatal("Unexpected should fail to load empty JSON")
	}

	_, err = LoadConfig[myStruct]("test-non-exist.json")
	if err == nil {
		t.Fatal("Unexpected should fail to Load non-existent config")
	}

	_, err = LoadConfig[myStruct]("test.json")
	if err == nil {
		t.Fatal("Unexpected should fail to load empty JSON")
	}

	// defer os.Remove("test.json")
	saveMe = myStruct{"1", "guest", "nopassword", []string{"Work", "Documents", "Music"}}
	err = SaveConfig(saveMe, "test.json")
	if err != nil {
		t.Fatal(err)
	}
	saveMe1, err := LoadConfig[myStruct]("test.json")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saveMe1, &saveMe) {
		t.Fatalf("Expected %v, got %v", saveMe1, saveMe)
	}
}

func TestSaveLoad(t *testing.T) {
	defer os.RemoveAll("test.json") //nolint:errcheck
	type myStruct struct {
		Version     string
		User        string
		Password    string
		Directories []string
	}
	saveMe := myStruct{"1", "guest", "nopassword", []string{"Work", "Documents", "Music"}}
	err := SaveConfig(&saveMe, "test.json")
	if err != nil {
		t.Fatal(err)
	}

	loadMe, err := LoadConfig[myStruct]("test.json")
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(&saveMe, loadMe) {
		t.Fatalf("Expected %v, got %v", &saveMe, loadMe)
	}

	mismatch := myStruct{"1.1", "guest", "nopassword", []string{"Work", "Documents", "Music"}}
	if reflect.DeepEqual(&saveMe, &mismatch) {
		t.Fatal("Expected to mismatch but succeeded instead")
	}
}

func TestSaveLoadYAML(t *testing.T) {
	t.Parallel()

	type myStruct struct {
		Version string   `yaml:"version"`
		Roots   []string `yaml:"roots"`
	}

	filename := "test.yaml"
	defer os.Remove(filename) //nolint:errcheck

	saveMe := myStruct{
		Version: "1",
		Roots:   []string{"/tmp/a", "/tmp/b"},
	}

	if err := SaveConfig(&saveMe, filename); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "version:") {
		t.Fatalf("SaveConfig() expected YAML output, got: %s", string(data))
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		t.Fatalf("SaveConfig() expected YAML output, got JSON: %s", string(data))
	}

	loadMe, err := LoadConfig[myStruct](filename)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if !reflect.DeepEqual(&saveMe, loadMe) {
		t.Fatalf("Expected %v, got %v", &saveMe, loadMe)
	}

	rawYAML := "version: \"1\"\nroots:\n  - /tmp/c\n"
	if err := os.WriteFile(filename, []byte(rawYAML), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loadMe, err = LoadConfig[myStruct](filename)
	if err != nil {
		t.Fatalf("LoadConfig() YAML parse error = %v", err)
	}
	if !reflect.DeepEqual(loadMe.Roots, []string{"/tmp/c"}) {
		t.Fatalf("LoadConfig() roots mismatch: got %v", loadMe.Roots)
	}
}
