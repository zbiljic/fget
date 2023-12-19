package gitexec

import (
	"errors"
	"fmt"
	"os/exec"
)

type FetchOptions struct {
	CmdDir string

	All                      bool
	Append                   bool
	Atomic                   bool
	Depth                    int
	Deepen                   int
	ShallowSince             string
	ShallowExclude           string
	Unshallow                bool
	UpdateShallow            bool
	NegotiationTip           string
	NegotiateOnly            bool
	DryRun                   bool
	Porcelain                bool
	WriteFetchHead           bool
	Force                    bool
	Keep                     bool
	Multiple                 bool
	AutoMaintenance          bool
	AutoGc                   bool
	WriteCommitGraph         bool
	Prefetch                 bool
	Prune                    bool
	PruneTags                bool
	NoTags                   bool
	Refetch                  bool
	Refmap                   string
	Tags                     bool
	RecurseSubmodules        string
	Jobs                     int
	NoRecurseSubmodules      bool
	SetUpstream              bool
	SubmodulePrefix          string
	RecurseSubmodulesDefault string
	UpdateHeadOk             bool
	UploadPack               string
	Quiet                    bool
	Verbose                  bool
	Progress                 bool
	ServerOption             []string
	ShowForcedUpdates        bool
	NoShowForcedUpdates      bool
	IPv4                     bool
	IPv6                     bool

	Repository string
	Group      string
	Refspec    string
}

func FetchCmd(opts *FetchOptions) *exec.Cmd {
	args := []string{"fetch"}

	if opts.All {
		args = append(args, "--all")
	}
	if opts.Append {
		args = append(args, "--append")
	}
	if opts.Atomic {
		args = append(args, "--atomic")
	}
	if opts.Depth > 0 {
		args = append(args, fmt.Sprintf("--depth=%d", opts.Depth))
	}
	if opts.Deepen > 0 {
		args = append(args, fmt.Sprintf("--deepen=%d", opts.Deepen))
	}
	if opts.ShallowSince != "" {
		args = append(args, fmt.Sprintf("--shallow-since=%s", opts.ShallowSince))
	}
	if opts.ShallowExclude != "" {
		args = append(args, fmt.Sprintf("--shallow-exclude=%s", opts.ShallowExclude))
	}
	if opts.Unshallow {
		args = append(args, "--unshallow")
	}
	if opts.UpdateShallow {
		args = append(args, "--update-shallow")
	}
	if opts.NegotiationTip != "" {
		args = append(args, fmt.Sprintf("--negotiation-tip=%s", opts.NegotiationTip))
	}
	if opts.NegotiateOnly {
		args = append(args, "--negotiate-only")
	}
	if opts.DryRun {
		args = append(args, "--dry-run")
	}
	if opts.Porcelain {
		args = append(args, "--porcelain")
	}
	if opts.WriteFetchHead {
		args = append(args, "--write-fetch-head")
	}
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Keep {
		args = append(args, "--keep")
	}
	if opts.Multiple {
		args = append(args, "--multiple")
	}
	if opts.AutoMaintenance {
		args = append(args, "--auto-maintenance")
	}
	if opts.AutoGc {
		args = append(args, "--auto-gc")
	}
	if opts.WriteCommitGraph {
		args = append(args, "--write-commit-graph")
	}
	if opts.Prefetch {
		args = append(args, "--prefetch")
	}
	if opts.Prune {
		args = append(args, "--prune")
	}
	if opts.PruneTags {
		args = append(args, "--prune-tags")
	}
	if opts.NoTags {
		args = append(args, "--no-tags")
	}
	if opts.Refetch {
		args = append(args, "--refetch")
	}
	if opts.Refmap != "" {
		args = append(args, fmt.Sprintf("--refmap=%s", opts.Refmap))
	}
	if opts.Tags {
		args = append(args, "--tags")
	}
	if opts.RecurseSubmodules != "" {
		args = append(args, fmt.Sprintf("--recurse-submodules=%s", opts.RecurseSubmodules))
	}
	if opts.Jobs > 0 {
		args = append(args, fmt.Sprintf("--jobs=%d", opts.Jobs))
	}
	if opts.NoRecurseSubmodules {
		args = append(args, "--no-recurse-submodules")
	}
	if opts.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if opts.SubmodulePrefix != "" {
		args = append(args, fmt.Sprintf("--submodule-prefix=%s", opts.SubmodulePrefix))
	}
	if opts.RecurseSubmodulesDefault != "" {
		args = append(args, fmt.Sprintf("--recurse-submodules-default=%s", opts.RecurseSubmodulesDefault))
	}
	if opts.UpdateHeadOk {
		args = append(args, "--update-head-ok")
	}
	if opts.UploadPack != "" {
		args = append(args, fmt.Sprintf("--upload-pack %s", opts.UploadPack))
	}
	if opts.Quiet {
		args = append(args, "--quiet")
	}
	if opts.Verbose {
		args = append(args, "--verbose")
	}
	if opts.Progress {
		args = append(args, "--progress")
	}
	if len(opts.ServerOption) > 0 {
		for _, serverOption := range opts.ServerOption {
			args = append(args, fmt.Sprintf("--server-option=%s", serverOption))
		}
	}
	if opts.ShowForcedUpdates {
		args = append(args, "--show-forced-updates")
	}
	if opts.NoShowForcedUpdates {
		args = append(args, "--no-show-forced-updates")
	}
	if opts.IPv4 {
		args = append(args, "--ipv4")
	}
	if opts.IPv6 {
		args = append(args, "--ipv6")
	}

	if opts.Repository != "" {
		args = append(args, opts.Repository)
	}
	if opts.Group != "" {
		args = append(args, opts.Group)
	}
	if opts.Refspec != "" {
		args = append(args, opts.Refspec)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = opts.CmdDir

	return cmd
}

func Fetch(opts *FetchOptions) ([]byte, error) {
	if opts.CmdDir == "" {
		return nil, errors.New("missing command working directory")
	}

	cmd := FetchCmd(opts)

	return run(cmd)
}
