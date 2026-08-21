package gitinspect

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

type stubRunner struct {
	results map[string]Result
	errors  map[string]error
}

func (runner stubRunner) Run(_ context.Context, _ string, args ...string) (Result, error) {
	key := strings.Join(args, "\x00")
	return runner.results[key], runner.errors[key]
}

func TestInspectState(t *testing.T) {
	t.Parallel()

	runner := stubRunner{results: map[string]Result{
		"rev-parse\x00--verify\x00-q\x00HEAD": {
			Stdout: "head-commit\n",
		},
		"symbolic-ref\x00-q\x00HEAD": {
			Stdout: "refs/heads/main\n",
		},
		"for-each-ref\x00--format=%(upstream)\x00refs/heads/main": {
			Stdout: "refs/remotes/origin/main\n",
		},
		"rev-parse\x00--verify\x00refs/remotes/origin/main": {
			Stdout: "upstream-commit\n",
		},
		"for-each-ref\x00--sort=refname\x00--format=%(refname)%00%(objectname)\x00refs": {
			Stdout: "refs/heads/main\x00head-commit\nrefs/remotes/origin/main\x00upstream-commit\nrefs/tags/v1\x00tag-commit\n",
		},
	}}

	state, err := InspectState(context.Background(), "/repo", runner)
	if err != nil {
		t.Fatalf("InspectState() error = %v", err)
	}
	if state.Head != (Reference{Ref: "refs/heads/main", Commit: "head-commit"}) {
		t.Fatalf("Head = %+v", state.Head)
	}
	if state.Upstream == nil || *state.Upstream != (Reference{Ref: "refs/remotes/origin/main", Commit: "upstream-commit"}) {
		t.Fatalf("Upstream = %+v", state.Upstream)
	}
	if state.LocalRefCount != 2 || !strings.HasPrefix(state.LocalRefsDigest, "sha256:") {
		t.Fatalf("local refs = %d, %q", state.LocalRefCount, state.LocalRefsDigest)
	}
}

func TestInspectStateAcceptsUnbornHead(t *testing.T) {
	t.Parallel()

	headKey := "rev-parse\x00--verify\x00-q\x00HEAD"
	runner := stubRunner{
		results: map[string]Result{},
		errors: map[string]error{
			headKey: &CommandError{ExitCode: 1, Err: errors.New("exit status 1")},
		},
	}

	state, err := InspectState(context.Background(), "/repo", runner)
	if err != nil {
		t.Fatalf("InspectState() error = %v", err)
	}
	if state != (State{}) {
		t.Fatalf("InspectState() = %+v, want empty state", state)
	}
}

func TestRefNamesAndGitDirPath(t *testing.T) {
	t.Parallel()

	runner := stubRunner{results: map[string]Result{
		"for-each-ref\x00--format=%(refname)\x00refs/remotes/origin": {
			Stdout: "refs/remotes/origin/HEAD\nrefs/remotes/origin/main\n",
		},
		"rev-parse\x00--git-dir": {Stdout: ".git\n"},
	}}

	refs, err := RefNames(context.Background(), "/repo", runner, "refs/remotes/origin")
	if err != nil {
		t.Fatalf("RefNames() error = %v", err)
	}
	if len(refs) != 2 || refs[0] != "refs/remotes/origin/HEAD" || refs[1] != "refs/remotes/origin/main" {
		t.Fatalf("RefNames() = %v", refs)
	}

	gitDir, err := GitDirPath(context.Background(), "/repo", runner)
	if err != nil {
		t.Fatalf("GitDirPath() error = %v", err)
	}
	if gitDir != filepath.Join("/repo", ".git") {
		t.Fatalf("GitDirPath() = %q", gitDir)
	}
}
