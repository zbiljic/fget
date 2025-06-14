package gitexec

import (
	"errors"
	"fmt"
	"os/exec"
)

type RevListOptions struct {
	CmdDir string

	Count     bool
	All       bool
	Branches  string
	Tags      bool
	Remotes   bool
	Glob      string
	Quiet     bool
	Objects   bool
	Object    string
	NotObject string
	Since     string
	Until     string
	MaxCount  int
	Skip      int
	Reverse   bool
	Topo      bool
	Date      string
	Merges    bool
	NoMerges  bool
	Min       bool
	Max       bool
	Format    string
	Abbrev    string

	Revision string
}

func RevListCmd(opts *RevListOptions) *exec.Cmd {
	args := []string{"rev-list"}

	if opts.Count {
		args = append(args, "--count")
	}
	if opts.All {
		args = append(args, "--all")
	}
	if opts.Branches != "" {
		args = append(args, fmt.Sprintf("--branches=%s", opts.Branches))
	}
	if opts.Tags {
		args = append(args, "--tags")
	}
	if opts.Remotes {
		args = append(args, "--remotes")
	}
	if opts.Glob != "" {
		args = append(args, fmt.Sprintf("--glob=%s", opts.Glob))
	}
	if opts.Quiet {
		args = append(args, "--quiet")
	}
	if opts.Objects {
		args = append(args, "--objects")
	}
	if opts.Object != "" {
		args = append(args, fmt.Sprintf("--objects=%s", opts.Object))
	}
	if opts.NotObject != "" {
		args = append(args, fmt.Sprintf("--not=%s", opts.NotObject))
	}
	if opts.Since != "" {
		args = append(args, fmt.Sprintf("--since=%s", opts.Since))
	}
	if opts.Until != "" {
		args = append(args, fmt.Sprintf("--until=%s", opts.Until))
	}
	if opts.MaxCount > 0 {
		args = append(args, fmt.Sprintf("--max-count=%d", opts.MaxCount))
	}
	if opts.Skip > 0 {
		args = append(args, fmt.Sprintf("--skip=%d", opts.Skip))
	}
	if opts.Reverse {
		args = append(args, "--reverse")
	}
	if opts.Topo {
		args = append(args, "--topo-order")
	}
	if opts.Date != "" {
		args = append(args, fmt.Sprintf("--date=%s", opts.Date))
	}
	if opts.Merges {
		args = append(args, "--merges")
	}
	if opts.NoMerges {
		args = append(args, "--no-merges")
	}
	if opts.Min {
		args = append(args, "--min-parents=1")
	}
	if opts.Max {
		args = append(args, "--max-parents=1")
	}
	if opts.Format != "" {
		args = append(args, fmt.Sprintf("--format=%s", opts.Format))
	}
	if opts.Abbrev != "" {
		args = append(args, fmt.Sprintf("--abbrev=%s", opts.Abbrev))
	}

	if opts.Revision != "" {
		args = append(args, opts.Revision)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = opts.CmdDir

	return cmd
}

func RevList(opts *RevListOptions) ([]byte, error) {
	if opts.CmdDir == "" {
		return nil, errors.New("missing command working directory")
	}

	cmd := RevListCmd(opts)

	return run(cmd)
}
