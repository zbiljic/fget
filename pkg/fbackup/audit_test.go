package fbackup

import (
	"testing"

	"github.com/zbiljic/fget/pkg/gitinspect"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		probe RepositoryProbe
		want  Classification
	}{
		{
			name: "recloneable requires verified reachable clean repo",
			probe: RepositoryProbe{
				RemoteState: RemoteStateReachable,
			},
			want: ClassificationRecloneable,
		},
		{
			name: "unknown when remote verification omitted",
			probe: RepositoryProbe{
				TrackedDirtyCount: 1,
				RemoteState:       RemoteStateUnchecked,
			},
			want: ClassificationUnknown,
		},
		{
			name: "delta with tracked changes",
			probe: RepositoryProbe{
				TrackedDirtyCount: 2,
				RemoteState:       RemoteStateReachable,
			},
			want: ClassificationDelta,
		},
		{
			name: "delta with untracked files",
			probe: RepositoryProbe{
				UntrackedCount: 1,
				RemoteState:    RemoteStateReachable,
			},
			want: ClassificationDelta,
		},
		{
			name: "delta with local-only commits",
			probe: RepositoryProbe{
				LocalOnlyCommitCount: 3,
				RemoteState:          RemoteStateReachable,
			},
			want: ClassificationDelta,
		},
		{
			name: "full with local lfs objects",
			probe: RepositoryProbe{
				HasLocalLFSObjects: true,
				RemoteState:        RemoteStateReachable,
			},
			want: ClassificationFull,
		},
		{
			name: "full when remote not found",
			probe: RepositoryProbe{
				RemoteState: RemoteStateNotFound,
			},
			want: ClassificationFull,
		},
		{
			name: "problem with inspection error",
			probe: RepositoryProbe{
				RemoteState: RemoteStateReachable,
				Errors: []RepositoryError{{
					Code:      "git-status-failed",
					Operation: "status",
				}},
			},
			want: ClassificationProblem,
		},
		{
			name: "problem with auth error",
			probe: RepositoryProbe{
				RemoteState: RemoteStateAuthError,
			},
			want: ClassificationProblem,
		},
		{
			name: "problem with missing origin",
			probe: RepositoryProbe{
				RemoteState: RemoteStateMissing,
			},
			want: ClassificationProblem,
		},
		{
			name: "problem with submodules",
			probe: RepositoryProbe{
				RemoteState:   RemoteStateReachable,
				HasSubmodules: true,
			},
			want: ClassificationProblem,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Classify(tt.probe); got != tt.want {
				t.Fatalf("Classify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyUnknownRemoteNeverRecloneable(t *testing.T) {
	t.Parallel()

	probe := RepositoryProbe{
		RemoteState:          RemoteStateUnchecked,
		LocalOnlyCommitCount: 5,
		TrackedDirtyCount:    1,
	}

	if got := Classify(probe); got == ClassificationRecloneable {
		t.Fatalf("Classify() = %q, want any class except %q", got, ClassificationRecloneable)
	}
	if got := Classify(probe); got != ClassificationUnknown {
		t.Fatalf("Classify() = %q, want %q", got, ClassificationUnknown)
	}
}

func TestBuildRepositoryEntrySanitizesRemoteURL(t *testing.T) {
	t.Parallel()

	entry := BuildRepositoryEntry(RepositoryProbe{
		RemoteURL:   "https://user:token@example.com/acme/repo.git",
		RemoteState: RemoteStateUnchecked,
	})
	if entry.RemoteURL != "https://example.com/acme/repo.git" {
		t.Fatalf("RemoteURL = %q", entry.RemoteURL)
	}
}

func TestBuildRepositoryEntryCopiesGitState(t *testing.T) {
	t.Parallel()

	upstream := &gitinspect.Reference{Ref: "refs/remotes/origin/main", Commit: "upstream-commit"}
	probeErrors := []RepositoryError{{Code: "git-status-failed", Operation: "status", Message: "status unavailable"}}
	entry := BuildRepositoryEntry(RepositoryProbe{
		Git: gitinspect.State{
			Head:            gitinspect.Reference{Ref: "refs/heads/main", Commit: "head-commit"},
			Upstream:        upstream,
			LocalRefsDigest: "sha256:digest",
			LocalRefCount:   2,
		},
		Errors:      probeErrors,
		RemoteState: RemoteStateUnchecked,
	})

	upstream.Commit = "changed-after-build"
	probeErrors[0].Message = "changed-after-build"
	if entry.Git.Head.Commit != "head-commit" || entry.Git.Head.Ref != "refs/heads/main" {
		t.Fatalf("Git.Head = %+v", entry.Git.Head)
	}
	if entry.Git.Upstream == nil || entry.Git.Upstream.Commit != "upstream-commit" {
		t.Fatalf("Git.Upstream = %+v", entry.Git.Upstream)
	}
	if entry.Git.LocalRefsDigest != "sha256:digest" || entry.Git.LocalRefCount != 2 {
		t.Fatalf("Git state = %+v", entry.Git)
	}
	if len(entry.Errors) != 1 || entry.Errors[0].Message != "status unavailable" {
		t.Fatalf("Errors = %+v", entry.Errors)
	}
}

func TestBuildRepositoryEntryDropsMalformedCredentialBearingRemoteURL(t *testing.T) {
	t.Parallel()

	entry := BuildRepositoryEntry(RepositoryProbe{
		RemoteURL:   "https://user:secret%zz@example.com/acme/repo.git",
		RemoteState: RemoteStateUnchecked,
	})
	if entry.RemoteURL != "" {
		t.Fatalf("RemoteURL = %q, want empty string", entry.RemoteURL)
	}
}

func TestClassifyUncheckedRemoteWithLocalLFSObjectsIsFull(t *testing.T) {
	t.Parallel()

	probe := RepositoryProbe{
		RemoteState:        RemoteStateUnchecked,
		HasLocalLFSObjects: true,
	}

	if got := Classify(probe); got != ClassificationFull {
		t.Fatalf("Classify() = %q, want %q", got, ClassificationFull)
	}
}
