package fbackup

import "github.com/zbiljic/fget/pkg/giturl"

func BuildRepositoryEntry(probe RepositoryProbe) RepositoryEntry {
	reasons := classifyReasonCodes(probe)
	gitState := probe.Git
	if probe.Git.Upstream != nil {
		upstream := *probe.Git.Upstream
		gitState.Upstream = &upstream
	}
	return RepositoryEntry{
		ID:                   probe.ID,
		Path:                 probe.Path,
		RemoteURL:            giturl.Sanitize(probe.RemoteURL),
		Classification:       Classify(probe),
		Git:                  gitState,
		ReasonCodes:          reasons,
		TrackedDirtyCount:    probe.TrackedDirtyCount,
		UntrackedCount:       probe.UntrackedCount,
		UntrackedBytes:       probe.UntrackedBytes,
		LocalOnlyCommitCount: probe.LocalOnlyCommitCount,
		RemoteState:          probe.RemoteState,
		RemoteReason:         probe.RemoteReason,
		HasLFSAttributes:     probe.HasLFSAttributes,
		HasLocalLFSObjects:   probe.HasLocalLFSObjects,
		HasSubmodules:        probe.HasSubmodules,
		EstimatedSourceBytes: probe.EstimatedSourceBytes,
		Errors:               append([]RepositoryError(nil), probe.Errors...),
	}
}

func Classify(probe RepositoryProbe) Classification {
	if len(probe.Errors) > 0 {
		return ClassificationProblem
	}

	if probe.HasLocalLFSObjects {
		return ClassificationFull
	}

	if probe.HasSubmodules {
		return ClassificationProblem
	}

	if probe.RemoteState == "" {
		probe.RemoteState = RemoteStateUnchecked
	}

	if probe.RemoteState == RemoteStateUnchecked {
		return ClassificationUnknown
	}

	switch probe.RemoteState {
	case RemoteStateReachable:
		if probe.LocalOnlyCommitCount > 0 || probe.TrackedDirtyCount > 0 || probe.UntrackedCount > 0 {
			return ClassificationDelta
		}
		return ClassificationRecloneable
	case RemoteStateNotFound:
		return ClassificationFull
	case RemoteStateAuthError, RemoteStateNetworkError, RemoteStateTimeout, RemoteStateMissing:
		return ClassificationProblem
	default:
		return ClassificationProblem
	}
}

func classifyReasonCodes(probe RepositoryProbe) []string {
	reasons := make([]string, 0, 8)
	if len(probe.Errors) > 0 {
		reasons = append(reasons, "inspection-error")
	}

	switch probe.RemoteState {
	case "", RemoteStateUnchecked:
		reasons = append(reasons, "remote-unchecked")
	case RemoteStateReachable:
		reasons = append(reasons, "remote-reachable")
	case RemoteStateNotFound:
		reasons = append(reasons, "remote-not-found")
	case RemoteStateAuthError:
		reasons = append(reasons, "remote-auth-error")
	case RemoteStateNetworkError:
		reasons = append(reasons, "remote-network-error")
	case RemoteStateTimeout:
		reasons = append(reasons, "remote-timeout")
	case RemoteStateMissing:
		reasons = append(reasons, "remote-missing")
	default:
		reasons = append(reasons, "remote-unknown")
	}

	if probe.HasSubmodules {
		reasons = append(reasons, "submodules-present")
	}
	if probe.HasLFSAttributes {
		reasons = append(reasons, "lfs-configured")
	}
	if probe.HasLocalLFSObjects {
		reasons = append(reasons, "local-lfs-objects")
	}
	if probe.LocalOnlyCommitCount > 0 {
		reasons = append(reasons, "local-only-commits")
	}
	if probe.TrackedDirtyCount > 0 {
		reasons = append(reasons, "tracked-dirty")
	}
	if probe.UntrackedCount > 0 {
		reasons = append(reasons, "untracked-files")
	}

	return reasons
}
