package gitexec

import (
	"errors"
	"fmt"
	"os/exec"
)

type PullOptions struct {
	CmdDir string

	Quiet                   bool
	Verbose                 bool
	RecurseSubmodules       string
	Commit                  bool
	NoCommit                bool
	Edit                    bool
	NoEdit                  bool
	Cleanup                 string
	FfOnly                  bool
	Ff                      bool
	NoFf                    bool
	GpgSign                 string
	NoGpgSign               bool
	Log                     string
	NoLog                   bool
	Signoff                 string
	NoSignoff               bool
	Stat                    string
	NoStat                  bool
	Squash                  string
	NoSquash                bool
	Verify                  string
	NoVerify                bool
	Strategy                string
	StrategyOption          []string
	VerifySignatures        bool
	NoVerifySignatures      bool
	Summary                 bool
	NoSummary               bool
	Autostash               bool
	NoAutostash             bool
	AllowUnrelatedHistories bool
	Rebase                  string
	NoRebase                bool

	All                 bool
	Append              bool
	Atomic              bool
	Depth               int
	Deepen              int
	ShallowSince        string
	ShallowExclude      string
	Unshallow           bool
	UpdateShallow       bool
	NegotiationTip      string
	NegotiateOnly       bool
	DryRun              bool
	Porcelain           bool
	Force               bool
	Keep                bool
	Prefetch            bool
	Prune               bool
	NoTags              bool
	Refmap              string
	Tags                bool
	Jobs                int
	SetUpstream         bool
	UploadPack          string
	Progress            bool
	ServerOption        []string
	ShowForcedUpdates   bool
	NoShowForcedUpdates bool
	IPv4                bool
	IPv6                bool

	Repository string
	Refspec    string
}

func PullCmd(opts *PullOptions) *exec.Cmd {
	args := []string{"pull"}

	if opts.Quiet {
		args = append(args, "--quiet")
	}
	if opts.Verbose {
		args = append(args, "--verbose")
	}
	if opts.RecurseSubmodules != "" {
		args = append(args, fmt.Sprintf("--recurse-submodules=%s", opts.RecurseSubmodules))
	}
	if opts.Commit {
		args = append(args, "--commit")
	}
	if opts.NoCommit {
		args = append(args, "--no-commit")
	}
	if opts.Edit {
		args = append(args, "--edit")
	}
	if opts.NoEdit {
		args = append(args, "--no-edit")
	}
	if opts.Cleanup != "" {
		args = append(args, fmt.Sprintf("--cleanup=%s", opts.Cleanup))
	}
	if opts.FfOnly {
		args = append(args, "--ff-only")
	}
	if opts.Ff {
		args = append(args, "--ff")
	}
	if opts.NoFf {
		args = append(args, "--no-ff")
	}
	if opts.GpgSign != "" {
		args = append(args, fmt.Sprintf("--gpg-sign=%s", opts.GpgSign))
	}
	if opts.NoGpgSign {
		args = append(args, "--no-gpg-sign")
	}
	if opts.Log != "" {
		args = append(args, fmt.Sprintf("--log=%s", opts.Log))
	}
	if opts.NoLog {
		args = append(args, "--no-log")
	}
	if opts.Signoff != "" {
		args = append(args, fmt.Sprintf("--signoff=%s", opts.Signoff))
	}
	if opts.NoSignoff {
		args = append(args, "--no-signoff")
	}
	if opts.Stat != "" {
		args = append(args, fmt.Sprintf("--stat=%s", opts.Stat))
	}
	if opts.NoStat {
		args = append(args, "--no-stat")
	}
	if opts.Squash != "" {
		args = append(args, fmt.Sprintf("--squash=%s", opts.Squash))
	}
	if opts.NoSquash {
		args = append(args, "--no-squash")
	}
	if opts.Verify != "" {
		args = append(args, fmt.Sprintf("--verify=%s", opts.Verify))
	}
	if opts.NoVerify {
		args = append(args, "--no-verify")
	}
	if opts.Strategy != "" {
		args = append(args, fmt.Sprintf("--strategy=%s", opts.Strategy))
	}
	if len(opts.StrategyOption) > 0 {
		for _, strategyOption := range opts.StrategyOption {
			args = append(args, fmt.Sprintf("--strategy-option=%s", strategyOption))
		}
	}
	if opts.VerifySignatures {
		args = append(args, "--verify-signatures")
	}
	if opts.NoVerifySignatures {
		args = append(args, "--no-verify-signatures")
	}
	if opts.Summary {
		args = append(args, "--summary")
	}
	if opts.NoSummary {
		args = append(args, "--no-summary")
	}
	if opts.Autostash {
		args = append(args, "--autostash")
	}
	if opts.NoAutostash {
		args = append(args, "--no-autostash")
	}
	if opts.AllowUnrelatedHistories {
		args = append(args, "--allow-unrelated-histories")
	}
	if opts.Rebase != "" {
		args = append(args, fmt.Sprintf("--rebase=%s", opts.Rebase))
	}
	if opts.NoRebase {
		args = append(args, "--no-rebase")
	}

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
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Keep {
		args = append(args, "--keep")
	}
	if opts.Prefetch {
		args = append(args, "--prefetch")
	}
	if opts.Prune {
		args = append(args, "--prune")
	}
	if opts.NoTags {
		args = append(args, "--no-tags")
	}
	if opts.Refmap != "" {
		args = append(args, fmt.Sprintf("--refmap=%s", opts.Refmap))
	}
	if opts.Tags {
		args = append(args, "--tags")
	}
	if opts.Jobs > 0 {
		args = append(args, fmt.Sprintf("--jobs=%d", opts.Jobs))
	}
	if opts.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if opts.UploadPack != "" {
		args = append(args, fmt.Sprintf("--upload-pack %s", opts.UploadPack))
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
	if opts.Refspec != "" {
		args = append(args, opts.Refspec)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = opts.CmdDir

	return cmd
}

func Pull(opts *PullOptions) ([]byte, error) {
	if opts.CmdDir == "" {
		return nil, errors.New("missing command working directory")
	}

	cmd := PullCmd(opts)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, err
	}

	return out, nil
}
