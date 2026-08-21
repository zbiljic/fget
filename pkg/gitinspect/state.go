package gitinspect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
)

// State is a compact snapshot of identity-relevant repository state.
type State struct {
	Head            Reference  `json:"head"`
	Upstream        *Reference `json:"upstream,omitempty"`
	LocalRefsDigest string     `json:"local_refs_digest,omitempty"`
	LocalRefCount   int        `json:"local_ref_count"`
}

// Reference identifies a Git ref and the commit it resolves to.
type Reference struct {
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit,omitempty"`
}

// InspectState returns HEAD, its configured upstream, and a digest of local refs.
func InspectState(ctx context.Context, repoPath string, runner Runner) (State, error) {
	state := State{}

	headOut, err := runner.Run(ctx, repoPath, "rev-parse", "--verify", "-q", "HEAD")
	if err != nil {
		if gitErr, ok := err.(*CommandError); !ok || gitErr.ExitCode != 1 {
			return state, err
		}
	} else {
		state.Head.Commit = strings.TrimSpace(headOut.Stdout)
	}

	if state.Head.Commit != "" {
		refOut, refErr := runner.Run(ctx, repoPath, "symbolic-ref", "-q", "HEAD")
		if refErr != nil {
			if gitErr, ok := refErr.(*CommandError); !ok || gitErr.ExitCode != 1 {
				return state, refErr
			}
		} else {
			state.Head.Ref = strings.TrimSpace(refOut.Stdout)
		}

		if state.Head.Ref != "" {
			upstreamOut, upstreamErr := runner.Run(ctx, repoPath, "for-each-ref", "--format=%(upstream)", state.Head.Ref)
			if upstreamErr != nil {
				return state, upstreamErr
			}
			upstreamRef := strings.TrimSpace(upstreamOut.Stdout)
			if upstreamRef != "" {
				commitOut, commitErr := runner.Run(ctx, repoPath, "rev-parse", "--verify", upstreamRef)
				if commitErr != nil {
					return state, commitErr
				}
				state.Upstream = &Reference{
					Ref:    upstreamRef,
					Commit: strings.TrimSpace(commitOut.Stdout),
				}
			}
		}
	}

	refsOut, err := runner.Run(ctx, repoPath, "for-each-ref", "--sort=refname", "--format=%(refname)%00%(objectname)", "refs")
	if err != nil {
		return state, err
	}
	localRefs := make([]string, 0, 8)
	for _, line := range strings.Split(refsOut.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 2)
		if len(parts) != 2 {
			return state, errors.New("malformed local ref snapshot")
		}
		if strings.HasPrefix(parts[0], "refs/remotes/") {
			continue
		}
		localRefs = append(localRefs, parts[0]+"\x00"+parts[1]+"\n")
	}
	state.LocalRefCount = len(localRefs)
	if len(localRefs) > 0 {
		digest := sha256.Sum256([]byte(strings.Join(localRefs, "")))
		state.LocalRefsDigest = "sha256:" + hex.EncodeToString(digest[:])
	}

	return state, nil
}

// RefNames lists matching refs in the order returned by Git.
func RefNames(ctx context.Context, repoPath string, runner Runner, pattern string) ([]string, error) {
	out, err := runner.Run(ctx, repoPath, "for-each-ref", "--format=%(refname)", pattern)
	if err != nil {
		return nil, err
	}

	refs := make([]string, 0, 8)
	for _, line := range strings.Split(out.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		refs = append(refs, line)
	}

	return refs, nil
}

// GitDirPath resolves the repository's Git directory to an absolute path.
func GitDirPath(ctx context.Context, repoPath string, runner Runner) (string, error) {
	out, err := runner.Run(ctx, repoPath, "rev-parse", "--git-dir")
	if err != nil {
		return "", err
	}

	gitDirPath := strings.TrimSpace(out.Stdout)
	if gitDirPath == "" {
		return "", errors.New("empty git dir path")
	}
	if filepath.IsAbs(gitDirPath) {
		return filepath.Clean(gitDirPath), nil
	}

	gitDirPath, err = filepath.Abs(filepath.Join(repoPath, gitDirPath))
	if err != nil {
		return "", err
	}
	return filepath.Clean(gitDirPath), nil
}
