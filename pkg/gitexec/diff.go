package gitexec

import (
	"errors"
	"os/exec"
)

type DiffOptions struct {
	CmdDir string

	Cached    bool
	MergeBase bool
	NoIndex   bool

	Commit []string
	Path   []string
}

func DiffCmd(opts *DiffOptions) *exec.Cmd {
	args := []string{"diff"}

	if opts.Cached {
		args = append(args, "--cached")
	}
	if opts.MergeBase {
		args = append(args, "--merge-base")
	}
	if opts.NoIndex {
		args = append(args, "--no-index")
	}

	if len(opts.Commit) > 0 {
		args = append(args, opts.Commit...)
	}
	if len(opts.Path) > 0 {
		args = append(args, "--")
		args = append(args, opts.Path...)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = opts.CmdDir

	return cmd
}

func Diff(opts *DiffOptions) ([]byte, error) {
	if opts.CmdDir == "" {
		return nil, errors.New("missing command working directory")
	}

	cmd := DiffCmd(opts)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, err
	}

	return out, nil
}
