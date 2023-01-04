package cmd

import (
	"fmt"
	"io"
	"os"
)

const (
	// ExitCodeOK is a 0 exit code.
	ExitCodeOK = iota
	// ExitCodeErr is a generic exit code, for all non-special errors.
	ExitCodeErr
)

// ErrExit writes an error to the given io.Writer & exits.
func ErrExit(w io.Writer, err error) {
	fmt.Fprint(w, "Error: ")
	fmt.Fprintln(w, err.Error())

	os.Exit(ExitCodeErr)
}

// ExitIfErr only calls ErrExit if there is an error present.
func ExitIfErr(w io.Writer, err error) {
	if err != nil {
		ErrExit(w, err)
	}
}

// GetWd is a convenience method to get the working directory.
func GetWd() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting working directory: %s", err.Error())
		os.Exit(ExitCodeErr)
	}

	return dir
}
