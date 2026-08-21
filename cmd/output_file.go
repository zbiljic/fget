package cmd

import (
	"io"
	"os"
	"path/filepath"
)

func writeAtomicOutputFile(path, tempPattern string, write func(io.Writer) error) (err error) {
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), tempPattern)
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()

	if err := write(temp); err != nil {
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
