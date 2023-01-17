package cmd

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/pterm/pterm"
)

var (
	// isNotTerminal defines if the output is going into terminal or not.
	// It's dynamically set to false or true based on the stdout's file
	// descriptor referring to a terminal or not.
	isNotTerminal = os.Getenv("TERM") == "dumb" ||
		(!isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()))

	// dynamicOutput defines the standard output of the dynamic output.
	// By default os.Stdout is used unless output is not going into terminal,
	// in which case os.Stderr is used.
	dynamicOutput io.Writer = os.Stdout
)

func init() {
	// configuration for non-terminal output
	if isNotTerminal {
		pterm.DisableColor()

		dynamicOutput = os.Stderr
	}
}

var (
	ptermInfoMessageStyle    = pterm.NewStyle(pterm.ThemeDefault.InfoMessageStyle...)
	ptermSuccessMessageStyle = pterm.NewStyle(pterm.ThemeDefault.SuccessMessageStyle...)
	ptermErrorMessageStyle   = pterm.NewStyle(pterm.ThemeDefault.ErrorMessageStyle...)
	ptermScopeStyle          = pterm.NewStyle(pterm.ThemeDefault.ScopeStyle...)
)

func ptermWarningWithPrefixText(text string) *pterm.PrefixPrinter {
	return pterm.Warning.WithPrefix(pterm.Prefix{
		Style: pterm.Warning.Prefix.Style,
		Text:  text,
	})
}

func ptermSuccessWithPrefixText(text string) *pterm.PrefixPrinter {
	return pterm.Success.WithPrefix(pterm.Prefix{
		Style: pterm.Success.Prefix.Style,
		Text:  text,
	})
}
