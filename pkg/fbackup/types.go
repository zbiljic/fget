package fbackup

import (
	"time"

	"github.com/zbiljic/fget/pkg/gitinspect"
)

const ManifestVersion = "1"

type Classification string

const (
	ClassificationRecloneable Classification = "recloneable"
	ClassificationDelta       Classification = "delta"
	ClassificationFull        Classification = "full"
	ClassificationProblem     Classification = "problem"
	ClassificationUnknown     Classification = "unknown"
)

type RemoteState string

const (
	RemoteStateUnchecked    RemoteState = "unchecked"
	RemoteStateReachable    RemoteState = "reachable"
	RemoteStateNotFound     RemoteState = "not-found"
	RemoteStateAuthError    RemoteState = "auth-error"
	RemoteStateNetworkError RemoteState = "network-error"
	RemoteStateTimeout      RemoteState = "timeout"
	RemoteStateMissing      RemoteState = "missing"
)

type Manifest struct {
	Version      string            `json:"version"`
	GeneratedAt  time.Time         `json:"generated_at"`
	Roots        []string          `json:"roots"`
	Catalog      *CatalogIdentity  `json:"catalog,omitempty"`
	Repositories []RepositoryEntry `json:"repositories"`
}

type CatalogIdentity struct {
	Path   string `json:"path,omitempty"`
	Digest string `json:"digest"`
}

type RepositoryError struct {
	Code      string `json:"code"`
	Operation string `json:"operation"`
	Message   string `json:"message,omitempty"`
}

type RepositoryEntry struct {
	ID                   string            `json:"id"`
	Path                 string            `json:"path"`
	RemoteURL            string            `json:"remote_url,omitempty"`
	Classification       Classification    `json:"classification"`
	Git                  gitinspect.State  `json:"git"`
	ReasonCodes          []string          `json:"reason_codes"`
	TrackedDirtyCount    int               `json:"tracked_dirty_count"`
	UntrackedCount       int               `json:"untracked_count"`
	UntrackedBytes       int64             `json:"untracked_bytes"`
	LocalOnlyCommitCount int               `json:"local_only_commit_count"`
	RemoteState          RemoteState       `json:"remote_state"`
	RemoteReason         string            `json:"remote_reason,omitempty"`
	HasLFSAttributes     bool              `json:"has_lfs_attributes"`
	HasLocalLFSObjects   bool              `json:"has_local_lfs_objects"`
	HasSubmodules        bool              `json:"has_submodules"`
	EstimatedSourceBytes int64             `json:"estimated_source_bytes"`
	Errors               []RepositoryError `json:"errors,omitempty"`
}

type RepositoryProbe struct {
	ID                   string
	Path                 string
	RemoteURL            string
	Git                  gitinspect.State
	TrackedDirtyCount    int
	UntrackedCount       int
	UntrackedBytes       int64
	LocalOnlyCommitCount int
	RemoteState          RemoteState
	RemoteReason         string
	HasLFSAttributes     bool
	HasLocalLFSObjects   bool
	HasSubmodules        bool
	EstimatedSourceBytes int64
	Errors               []RepositoryError
}
