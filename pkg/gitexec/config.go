package gitexec

import (
	"errors"
	"os/exec"
)

type ConfigOptions struct {
	CmdDir string

	Global bool
	System bool
	Local  bool

	Unset      bool
	UnsetAll   bool
	List       bool
	FixedValue bool

	Name  string
	Value string
}

func ConfigCmd(opts *ConfigOptions) *exec.Cmd {
	args := []string{"config"}

	if opts.Global {
		args = append(args, "--global")
	}
	if opts.System {
		args = append(args, "--system")
	}
	if opts.Local {
		args = append(args, "--local")
	}

	if opts.Unset {
		args = append(args, "--unset")
	}
	if opts.UnsetAll {
		args = append(args, "--unset-all")
	}
	if opts.List {
		args = append(args, "--list")
	}
	if opts.FixedValue {
		args = append(args, "--fixed-value")
	}

	if opts.Name != "" {
		args = append(args, opts.Name)
	}
	if opts.Value != "" {
		args = append(args, opts.Value)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = opts.CmdDir

	return cmd
}

func Config(opts *ConfigOptions) ([]byte, error) {
	if opts.CmdDir == "" {
		return nil, errors.New("missing command working directory")
	}

	cmd := ConfigCmd(opts)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, err
	}

	return out, nil
}
