package gitexec

import (
	"errors"
	"fmt"
	"os/exec"
)

type GcOptions struct {
	CmdDir string

	Aggressive      bool
	Auto            bool
	Prune           string
	NoPrune         bool
	Quiet           bool
	Force           bool
	KeepLargestPack bool
}

func GcCmd(opts *GcOptions) *exec.Cmd {
	args := []string{"gc"}

	if opts.Aggressive {
		args = append(args, "--aggressive")
	}
	if opts.Auto {
		args = append(args, "--auto")
	}
	if opts.Prune != "" {
		args = append(args, fmt.Sprintf("--prune=%s", opts.Prune))
	}
	if opts.NoPrune {
		args = append(args, "--no-prune")
	}
	if opts.Quiet {
		args = append(args, "--quiet")
	}
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.KeepLargestPack {
		args = append(args, "--keep-largest-pack")
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = opts.CmdDir

	return cmd
}

func Gc(opts *GcOptions) ([]byte, error) {
	if opts.CmdDir == "" {
		return nil, errors.New("missing command working directory")
	}

	cmd := GcCmd(opts)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, err
	}

	return out, nil
}
