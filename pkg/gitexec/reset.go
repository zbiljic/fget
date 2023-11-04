package gitexec

import (
	"errors"
	"fmt"
	"os/exec"
)

type ResetOptions struct {
	CmdDir string

	Quiet            bool
	Refresh          bool
	NoRefresh        bool
	PathspecFromFile string
	PathspecFileNul  bool
	Pathspec         string

	Soft                bool
	Mixed               bool
	Hard                bool
	Merge               bool
	Keep                bool
	RecurseSubmodules   bool
	NoRecurseSubmodules bool
	Commit              string
}

func ResetCmd(opts *ResetOptions) *exec.Cmd {
	args := []string{"reset"}

	if opts.Quiet {
		args = append(args, "--quiet")
	}
	if opts.Refresh {
		args = append(args, "--refresh")
	}
	if opts.NoRefresh {
		args = append(args, "--no-refresh")
	}
	if opts.PathspecFromFile != "" {
		args = append(args, fmt.Sprintf("--pathspec-from-file=%s", opts.PathspecFromFile))
	}
	if opts.PathspecFileNul {
		args = append(args, "--pathspec-file-nul")
	}
	if opts.Pathspec != "" {
		args = append(args, opts.Pathspec)
	}

	if opts.Soft {
		args = append(args, "--soft")
	}
	if opts.Mixed {
		args = append(args, "--mixed")
	}
	if opts.Hard {
		args = append(args, "--hard")
	}
	if opts.Merge {
		args = append(args, "--merge")
	}
	if opts.Keep {
		args = append(args, "--keep")
	}
	if opts.RecurseSubmodules {
		args = append(args, "--recurse-submodules")
	}
	if opts.NoRecurseSubmodules {
		args = append(args, "--no-recurse-submodules")
	}
	if opts.Commit != "" {
		args = append(args, opts.Commit)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = opts.CmdDir

	return cmd
}

func Reset(opts *ResetOptions) ([]byte, error) {
	if opts.CmdDir == "" {
		return nil, errors.New("missing command working directory")
	}

	cmd := ResetCmd(opts)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, err
	}

	return out, nil
}
