package cmd

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/zbiljic/fget/pkg/fbackup"
	"github.com/zbiljic/fget/pkg/gitinspect"
)

type (
	backupGitRunner       = gitinspect.Runner
	backupGitOutput       = gitinspect.Result
	backupGitCommandError = gitinspect.CommandError
	backupGitCLI          = gitinspect.CLIRunner
)

var errBackupOriginRemoteMissing = errors.New("origin remote not configured")

type backupLocalProbe struct {
	RemoteURL            string
	HasOrigin            bool
	Git                  gitinspect.State
	TrackedDirtyCount    int
	UntrackedCount       int
	UntrackedBytes       int64
	LocalOnlyCommitCount int
	HasLFSAttributes     bool
	HasLocalLFSObjects   bool
	HasSubmodules        bool
	EstimatedSourceBytes int64
	Errors               []fbackup.RepositoryError
}

func inspectBackupLocalRepository(ctx context.Context, repoPath string, runner backupGitRunner) backupLocalProbe {
	probe := backupLocalProbe{}

	gitState, err := gitinspect.InspectState(ctx, repoPath, runner)
	if err != nil {
		probe.addError("git-state-failed", "git-state", "Git HEAD, upstream, or local refs could not be inspected")
	} else {
		probe.Git = gitState
	}

	remoteURL, hasOrigin, err := backupRemoteURL(ctx, repoPath, runner)
	probe.RemoteURL = remoteURL
	probe.HasOrigin = hasOrigin
	if err != nil {
		if errors.Is(err, errBackupOriginRemoteMissing) {
			probe.addError("origin-remote-missing", "remote-url", "origin remote is not configured")
		} else {
			probe.addError("remote-url-failed", "remote-url", "origin remote URL could not be inspected")
		}
	}

	trackedDirtyCount, untrackedCount, untrackedPaths, err := backupStatusSummary(ctx, repoPath, runner)
	if err != nil {
		probe.addError("git-status-failed", "status", "working tree and index status could not be inspected")
	} else {
		probe.TrackedDirtyCount = trackedDirtyCount
		probe.UntrackedCount = untrackedCount

		untrackedBytes, bytesErr := estimateRelativePathsBytes(ctx, repoPath, runner, untrackedPaths)
		if bytesErr != nil {
			probe.addError("untracked-size-failed", "untracked-size", "untracked file size could not be estimated")
		} else {
			probe.UntrackedBytes = untrackedBytes
		}
	}

	if probe.HasOrigin {
		localOnlyCount, countErr := backupLocalOnlyCommitCount(ctx, repoPath, runner)
		if countErr != nil {
			probe.addError("local-history-failed", "local-history", "local-only commits could not be counted")
		} else {
			probe.LocalOnlyCommitCount = localOnlyCount
		}
	}

	hasSubmodules, err := backupHasSubmodules(ctx, repoPath, runner)
	if err != nil {
		probe.addError("submodule-inspection-failed", "submodules", "submodule configuration could not be inspected")
	} else {
		probe.HasSubmodules = hasSubmodules
	}

	hasLFSAttributes, err := backupHasLFSAttributes(ctx, repoPath, runner)
	if err != nil {
		probe.addError("lfs-attributes-failed", "lfs-attributes", "Git LFS attributes could not be inspected")
	} else {
		probe.HasLFSAttributes = hasLFSAttributes
	}

	localLFSObjects, err := backupHasLocalLFSObjects(repoPath)
	if err != nil {
		probe.addError("lfs-objects-failed", "lfs-objects", "local Git LFS objects could not be inspected")
	} else {
		probe.HasLocalLFSObjects = localLFSObjects
	}

	estimatedBytes, err := estimateRepositoryBytes(ctx, repoPath, runner)
	if err != nil {
		probe.addError("source-size-failed", "source-size", "repository source size could not be estimated")
	} else {
		probe.EstimatedSourceBytes = estimatedBytes
	}

	return probe
}

func (probe *backupLocalProbe) addError(code, operation, message string) {
	probe.Errors = append(probe.Errors, fbackup.RepositoryError{
		Code:      code,
		Operation: operation,
		Message:   message,
	})
}

func verifyBackupRemote(ctx context.Context, repoPath string, runner backupGitRunner) (fbackup.RemoteState, string) {
	_, err := runner.Run(ctx, repoPath, "ls-remote", "--symref", "origin")
	if err == nil {
		return fbackup.RemoteStateReachable, ""
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fbackup.RemoteStateTimeout, "remote verification timed out"
	}

	if gitErr, ok := err.(*backupGitCommandError); ok {
		switch {
		case backupRemoteIsNotFound(gitErr):
			return fbackup.RemoteStateNotFound, "remote not found"
		case backupRemoteIsAuthError(gitErr):
			return fbackup.RemoteStateAuthError, "remote authentication failed"
		case backupRemoteIsTimeout(gitErr):
			return fbackup.RemoteStateTimeout, "remote verification timed out"
		default:
			return fbackup.RemoteStateNetworkError, "remote verification failed"
		}
	}

	return fbackup.RemoteStateNetworkError, "remote verification failed"
}

func backupRemoteFailureReason(err *backupGitCommandError, out backupGitOutput) string {
	if err == nil {
		return ""
	}

	reason := strings.TrimSpace(err.Stderr)
	if reason != "" {
		return reason
	}
	reason = strings.TrimSpace(out.Stderr)
	if reason != "" {
		return reason
	}
	reason = strings.TrimSpace(err.Stdout)
	if reason != "" {
		return reason
	}
	return strings.TrimSpace(err.Error())
}

func backupRemoteIsNotFound(err *backupGitCommandError) bool {
	message := strings.ToLower(backupRemoteFailureReason(err, backupGitOutput{}))
	return strings.Contains(message, "repository not found") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "does not appear to be a git repository") ||
		strings.Contains(message, "no such file or directory")
}

func backupRemoteIsAuthError(err *backupGitCommandError) bool {
	message := strings.ToLower(backupRemoteFailureReason(err, backupGitOutput{}))
	return strings.Contains(message, "authentication failed") ||
		strings.Contains(message, "permission denied") ||
		strings.Contains(message, "could not read username") ||
		strings.Contains(message, "terminal prompts disabled") ||
		strings.Contains(message, "publickey") ||
		strings.Contains(message, "access denied")
}

func backupRemoteIsTimeout(err *backupGitCommandError) bool {
	message := strings.ToLower(backupRemoteFailureReason(err, backupGitOutput{}))
	return strings.Contains(message, "timed out") || strings.Contains(message, "timeout")
}

func backupRemoteURL(ctx context.Context, repoPath string, runner backupGitRunner) (string, bool, error) {
	out, err := runner.Run(ctx, repoPath, "remote", "get-url", "origin")
	if err == nil {
		return strings.TrimSpace(out.Stdout), true, nil
	}

	if gitErr, ok := err.(*backupGitCommandError); ok {
		message := strings.ToLower(gitErr.Error())
		if strings.Contains(message, "no such remote") || strings.Contains(message, "not a git repository") {
			return "", false, errBackupOriginRemoteMissing
		}
	}

	return "", false, err
}

func backupStatusSummary(ctx context.Context, repoPath string, runner backupGitRunner) (int, int, []string, error) {
	out, err := runner.Run(ctx, repoPath, "status", "--porcelain=v1", "-z")
	if err != nil {
		return 0, 0, nil, err
	}

	var (
		trackedDirty int
		untracked    int
		paths        []string
	)

	data := []byte(out.Stdout)
	for index := 0; index < len(data); {
		if len(data[index:]) < 4 {
			return 0, 0, nil, errors.New("malformed status output")
		}

		status := string(data[index : index+2])
		index += 3

		end := bytes.IndexByte(data[index:], 0)
		if end < 0 {
			return 0, 0, nil, errors.New("malformed status path entry")
		}

		path := string(data[index : index+end])
		index += end + 1

		switch status {
		case "??":
			untracked++
			paths = append(paths, path)
		case "!!":
		default:
			trackedDirty++
		}

		if strings.Contains(status, "R") || strings.Contains(status, "C") {
			renameEnd := bytes.IndexByte(data[index:], 0)
			if renameEnd < 0 {
				return 0, 0, nil, errors.New("malformed rename status entry")
			}
			index += renameEnd + 1
		}
	}

	return trackedDirty, untracked, paths, nil
}

func backupLocalOnlyCommitCount(ctx context.Context, repoPath string, runner backupGitRunner) (int, error) {
	originRefs, err := gitinspect.RefNames(ctx, repoPath, runner, "refs/remotes/origin")
	if err != nil {
		return 0, err
	}
	relevantOriginRefs := originRefs[:0]
	for _, ref := range originRefs {
		if !strings.HasSuffix(ref, "/HEAD") {
			relevantOriginRefs = append(relevantOriginRefs, ref)
		}
	}
	originRefs = relevantOriginRefs
	if len(originRefs) == 0 {
		return 0, errors.New("origin has no local tracking refs")
	}

	localRefs, err := gitinspect.RefNames(ctx, repoPath, runner, "refs")
	if err != nil {
		return 0, err
	}

	relevantLocalRefs := make([]string, 0, len(localRefs)+1)
	seen := make(map[string]struct{}, len(localRefs)+1)
	for _, ref := range localRefs {
		if ref == "" || strings.HasPrefix(ref, "refs/remotes/origin/") || ref == "refs/remotes/origin" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		relevantLocalRefs = append(relevantLocalRefs, ref)
	}

	if out, headErr := runner.Run(ctx, repoPath, "rev-parse", "--verify", "HEAD"); headErr == nil {
		headRef := strings.TrimSpace(out.Stdout)
		if headRef != "" {
			if _, ok := seen["HEAD"]; !ok {
				relevantLocalRefs = append(relevantLocalRefs, "HEAD")
			}
		}
	}

	if len(relevantLocalRefs) == 0 {
		return 0, nil
	}

	sort.Strings(relevantLocalRefs)
	sort.Strings(originRefs)

	args := append([]string{"rev-list", "--count"}, relevantLocalRefs...)
	args = append(args, "--not")
	args = append(args, originRefs...)
	countOut, err := runner.Run(ctx, repoPath, args...)
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(countOut.Stdout))
	if err != nil {
		return 0, err
	}

	return count, nil
}

func backupHasSubmodules(ctx context.Context, repoPath string, runner backupGitRunner) (bool, error) {
	out, err := runner.Run(ctx, repoPath, "config", "--file", ".gitmodules", "--name-only", "--get-regexp", "^submodule\\..*\\.path$")
	if err == nil {
		return strings.TrimSpace(out.Stdout) != "", nil
	}

	if gitErr, ok := err.(*backupGitCommandError); ok {
		message := strings.ToLower(gitErr.Error())
		if gitErr.ExitCode == 1 || strings.Contains(message, "no such file or directory") {
			return false, nil
		}
	}

	return false, err
}

func backupHasLFSAttributes(ctx context.Context, repoPath string, runner backupGitRunner) (bool, error) {
	out, err := runner.Run(ctx, repoPath, "config", "--local", "--name-only", "--get-regexp", "^filter\\.lfs\\.")
	if err == nil && strings.TrimSpace(out.Stdout) != "" {
		return true, nil
	}
	if err != nil {
		if gitErr, ok := err.(*backupGitCommandError); !ok || gitErr.ExitCode != 1 {
			return false, err
		}
	}

	files, err := backupListedFiles(ctx, repoPath, runner, true)
	if err != nil {
		return false, err
	}

	for _, relativePath := range files {
		if filepath.Base(filepath.FromSlash(relativePath)) != ".gitattributes" {
			continue
		}

		content, readErr := os.ReadFile(filepath.Join(repoPath, filepath.FromSlash(relativePath)))
		if readErr != nil {
			return false, readErr
		}
		if bytes.Contains(content, []byte("filter=lfs")) {
			return true, nil
		}
	}

	return false, nil
}

func backupHasLocalLFSObjects(repoPath string) (bool, error) {
	path := filepath.Join(repoPath, ".git", "lfs", "objects")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}

	found := false
	walkErr := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if sizeInfo, statErr := entry.Info(); statErr == nil && sizeInfo.Size() >= 0 {
			found = true
			return filepath.SkipAll
		} else if statErr != nil {
			return statErr
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, filepath.SkipAll) {
		return false, walkErr
	}

	return found, nil
}

func estimateRelativePathsBytes(ctx context.Context, repoPath string, runner backupGitRunner, relativePaths []string) (int64, error) {
	files, err := backupListedFiles(ctx, repoPath, runner, false, relativePaths...)
	if err != nil {
		return 0, err
	}

	return estimateListedFilesBytes(repoPath, files)
}

func estimateRepositoryBytes(ctx context.Context, repoPath string, runner backupGitRunner) (int64, error) {
	trackedAndUntrackedFiles, err := backupListedFiles(ctx, repoPath, runner, true)
	if err != nil {
		return 0, err
	}

	worktreeBytes, err := estimateListedFilesBytes(repoPath, trackedAndUntrackedFiles)
	if err != nil {
		return 0, err
	}

	gitDirPath, err := gitinspect.GitDirPath(ctx, repoPath, runner)
	if err != nil {
		return 0, err
	}

	gitDirBytes, err := estimatePathBytes(gitDirPath)
	if err != nil {
		return 0, err
	}

	return worktreeBytes + gitDirBytes, nil
}

func backupListedFiles(ctx context.Context, repoPath string, runner backupGitRunner, includeTracked bool, relativePaths ...string) ([]string, error) {
	args := []string{"ls-files", "-z"}
	if includeTracked {
		args = append(args, "--cached")
	}
	args = append(args, "--others", "--exclude-standard", "--")
	if len(relativePaths) == 0 {
		args = append(args, ".")
	} else {
		args = append(args, relativePaths...)
	}

	out, err := runner.Run(ctx, repoPath, args...)
	if err != nil {
		return nil, err
	}

	return parseNULTerminatedPaths(out.Stdout), nil
}

func parseNULTerminatedPaths(stdout string) []string {
	if stdout == "" {
		return nil
	}

	seen := make(map[string]struct{})
	paths := make([]string, 0, 16)
	for _, entry := range strings.Split(stdout, "\x00") {
		if entry == "" {
			continue
		}
		entry = filepath.Clean(filepath.FromSlash(entry))
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		paths = append(paths, entry)
	}

	sort.Strings(paths)
	return paths
}

func estimateListedFilesBytes(root string, relativePaths []string) (int64, error) {
	var total int64
	for _, relativePath := range relativePaths {
		absolutePath := filepath.Join(root, relativePath)
		info, err := os.Lstat(absolutePath)
		if err != nil {
			return 0, err
		}
		if info.IsDir() {
			continue
		}
		total += info.Size()
	}

	return total, nil
}

func estimatePathBytes(path string) (int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}

	var total int64
	err = filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}

	return total, nil
}
