package gitexec

import (
	"errors"
	"fmt"
	"os/exec"
)

type LsRemoteOptions struct {
	CmdDir string

	Heads        bool
	Tags         bool
	Refs         bool
	Quiet        bool
	UploadPack   string
	ExitCode     bool
	GetUrl       bool
	Symref       bool
	SortKey      string
	ServerOption []string
	Repository   string
	References   []string
}

func LsRemoteCmd(opts *LsRemoteOptions) *exec.Cmd {
	args := []string{"ls-remote"}

	if opts.Heads {
		args = append(args, "--heads")
	}
	if opts.Tags {
		args = append(args, "--tags")
	}
	if opts.Refs {
		args = append(args, "--refs")
	}
	if opts.Quiet {
		args = append(args, "--quiet")
	}
	if opts.UploadPack != "" {
		args = append(args, fmt.Sprintf("--upload-pack=%s", opts.UploadPack))
	}
	if opts.ExitCode {
		args = append(args, "--exit-code")
	}
	if opts.GetUrl {
		args = append(args, "--get-url")
	}
	if opts.Symref {
		args = append(args, "--symref")
	}
	if opts.SortKey != "" {
		args = append(args, fmt.Sprintf("--sort=%s", opts.SortKey))
	}
	if len(opts.ServerOption) > 0 {
		for _, serverOption := range opts.ServerOption {
			args = append(args, fmt.Sprintf("--server-option=%s", serverOption))
		}
	}
	if opts.Repository != "" {
		args = append(args, opts.Repository)
	}
	if len(opts.References) > 0 {
		args = append(args, opts.References...)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = opts.CmdDir

	return cmd
}

func LsRemote(opts *LsRemoteOptions) ([]byte, error) {
	if opts.CmdDir == "" {
		return nil, errors.New("missing command working directory")
	}

	cmd := LsRemoteCmd(opts)

	return run(cmd)
}
