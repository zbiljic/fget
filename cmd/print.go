//nolint:unused
package cmd

import (
	"fmt"
	"io"

	"github.com/fatih/color"
)

var noPrompt = false

func setNoColor(noColor bool) {
	color.NoColor = noColor
}

func setNoPrompt(np bool) {
	noPrompt = np
}

func printInfo(w io.Writer, msg string, params ...interface{}) {
	fmt.Fprintln(w, fmt.Sprintf(msg, params...))
}

func printInfoNoEndline(w io.Writer, msg string, params ...interface{}) {
	fmt.Fprintf(w, msg, params...)
}

func printSuccess(w io.Writer, msg string, params ...interface{}) {
	fmt.Fprintln(w, color.New(color.FgGreen).Sprintf(msg, params...))
}

func printWarning(w io.Writer, msg string, params ...interface{}) {
	fmt.Fprintln(w, color.New(color.FgYellow).Sprintf(msg, params...))
}

func printWarningNoEndline(w io.Writer, msg string, params ...interface{}) {
	fmt.Fprint(w, color.New(color.FgYellow).Sprintf(msg, params...))
}

func printErr(w io.Writer, err error, params ...interface{}) {
	fmt.Fprintln(w, color.New(color.FgRed).Sprintf(err.Error(), params...))
}
