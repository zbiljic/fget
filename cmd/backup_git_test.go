package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zbiljic/fget/pkg/fbackup"
	"github.com/zbiljic/fget/pkg/gitinspect"
)

func TestBackupGitInspectRepositoryCases(t *testing.T) {
	t.Parallel()

	runner := backupGitCLI{}

	tests := []struct {
		name   string
		setup  func(t *testing.T) string
		assert func(t *testing.T, probe backupLocalProbe)
	}{
		{
			name: "clean",
			setup: func(t *testing.T) string {
				repoDir, remoteDir := initCommittedRepo(t)
				configureOrigin(t, repoDir, remoteDir)
				pushMain(t, repoDir)
				return repoDir
			},
			assert: func(t *testing.T, probe backupLocalProbe) {
				if len(probe.Errors) > 0 {
					t.Fatalf("Errors = %v", probe.Errors)
				}
				if probe.TrackedDirtyCount != 0 || probe.UntrackedCount != 0 || probe.LocalOnlyCommitCount != 0 {
					t.Fatalf("probe = %+v", probe)
				}
				if !probe.HasOrigin {
					t.Fatal("HasOrigin = false, want true")
				}
			},
		},
		{
			name: "dirty",
			setup: func(t *testing.T) string {
				repoDir, remoteDir := initCommittedRepo(t)
				configureOrigin(t, repoDir, remoteDir)
				pushMain(t, repoDir)
				writeTestFile(t, filepath.Join(repoDir, "tracked.txt"), "changed\n")
				return repoDir
			},
			assert: func(t *testing.T, probe backupLocalProbe) {
				if probe.TrackedDirtyCount == 0 {
					t.Fatalf("TrackedDirtyCount = %d, want > 0", probe.TrackedDirtyCount)
				}
			},
		},
		{
			name: "untracked",
			setup: func(t *testing.T) string {
				repoDir, remoteDir := initCommittedRepo(t)
				configureOrigin(t, repoDir, remoteDir)
				pushMain(t, repoDir)
				writeTestFile(t, filepath.Join(repoDir, "scratch.txt"), "scratch\n")
				return repoDir
			},
			assert: func(t *testing.T, probe backupLocalProbe) {
				if probe.UntrackedCount != 1 || probe.UntrackedBytes == 0 {
					t.Fatalf("probe = %+v", probe)
				}
			},
		},
		{
			name: "local-only",
			setup: func(t *testing.T) string {
				repoDir, remoteDir := initCommittedRepo(t)
				configureOrigin(t, repoDir, remoteDir)
				pushMain(t, repoDir)
				writeTestFile(t, filepath.Join(repoDir, "local-only.txt"), "local\n")
				gitRun(t, repoDir, "add", "local-only.txt")
				gitRun(t, repoDir, "commit", "-m", "local-only")
				return repoDir
			},
			assert: func(t *testing.T, probe backupLocalProbe) {
				if probe.LocalOnlyCommitCount != 1 {
					t.Fatalf("LocalOnlyCommitCount = %d, want 1", probe.LocalOnlyCommitCount)
				}
			},
		},
		{
			name: "local-only on non checked out branch",
			setup: func(t *testing.T) string {
				repoDir, remoteDir := initCommittedRepo(t)
				configureOrigin(t, repoDir, remoteDir)
				pushMain(t, repoDir)
				gitRun(t, repoDir, "checkout", "-b", "feature/local-only")
				writeTestFile(t, filepath.Join(repoDir, "branch-only.txt"), "branch-only\n")
				gitRun(t, repoDir, "add", "branch-only.txt")
				gitRun(t, repoDir, "commit", "-m", "branch only")
				gitRun(t, repoDir, "checkout", "main")
				return repoDir
			},
			assert: func(t *testing.T, probe backupLocalProbe) {
				if probe.LocalOnlyCommitCount != 1 {
					t.Fatalf("LocalOnlyCommitCount = %d, want 1", probe.LocalOnlyCommitCount)
				}
			},
		},
		{
			name: "detached-head",
			setup: func(t *testing.T) string {
				repoDir, remoteDir := initCommittedRepo(t)
				configureOrigin(t, repoDir, remoteDir)
				pushMain(t, repoDir)
				gitRun(t, repoDir, "checkout", "--detach", "HEAD")
				return repoDir
			},
			assert: func(t *testing.T, probe backupLocalProbe) {
				if len(probe.Errors) > 0 {
					t.Fatalf("Errors = %v", probe.Errors)
				}
			},
		},
		{
			name: "no-origin",
			setup: func(t *testing.T) string {
				repoDir, _ := initCommittedRepo(t)
				return repoDir
			},
			assert: func(t *testing.T, probe backupLocalProbe) {
				if probe.HasOrigin {
					t.Fatal("HasOrigin = true, want false")
				}
				if len(probe.Errors) != 1 {
					t.Fatalf("Errors = %v, want one origin error", probe.Errors)
				}
				if probe.Errors[0].Code != "origin-remote-missing" || probe.Errors[0].Operation != "remote-url" {
					t.Fatalf("Errors[0] = %+v", probe.Errors[0])
				}
			},
		},
		{
			name: "submodule-marker",
			setup: func(t *testing.T) string {
				repoDir, remoteDir := initCommittedRepo(t)
				configureOrigin(t, repoDir, remoteDir)
				pushMain(t, repoDir)
				writeTestFile(t, filepath.Join(repoDir, ".gitmodules"), "[submodule \"vendor/lib\"]\n\tpath = vendor/lib\n\turl = ../lib.git\n")
				return repoDir
			},
			assert: func(t *testing.T, probe backupLocalProbe) {
				if !probe.HasSubmodules {
					t.Fatal("HasSubmodules = false, want true")
				}
			},
		},
		{
			name: "lfs-marker",
			setup: func(t *testing.T) string {
				repoDir, remoteDir := initCommittedRepo(t)
				configureOrigin(t, repoDir, remoteDir)
				pushMain(t, repoDir)
				writeTestFile(t, filepath.Join(repoDir, ".gitattributes"), "*.bin filter=lfs diff=lfs merge=lfs -text\n")
				writeTestFile(t, filepath.Join(repoDir, ".git", "lfs", "objects", "ab", "cd", "object"), "blob\n")
				return repoDir
			},
			assert: func(t *testing.T, probe backupLocalProbe) {
				if !probe.HasLFSAttributes || !probe.HasLocalLFSObjects {
					t.Fatalf("probe = %+v", probe)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repoDir := tt.setup(t)
			beforeStatus := gitOutput(t, repoDir, "status", "--porcelain")
			indexPath := filepath.Join(repoDir, ".git", "index")
			beforeIndex := statModTime(t, indexPath)
			beforeRepo := statModTime(t, repoDir)

			restore := setReadOnlyTree(t, repoDir)
			defer restore()

			probe := inspectBackupLocalRepository(context.Background(), repoDir, runner)
			tt.assert(t, probe)

			afterStatus := gitOutput(t, repoDir, "status", "--porcelain")
			afterIndex := statModTime(t, indexPath)
			afterRepo := statModTime(t, repoDir)

			if beforeStatus != afterStatus {
				t.Fatalf("git status changed:\nbefore=%q\nafter=%q", beforeStatus, afterStatus)
			}
			if !beforeIndex.Equal(afterIndex) {
				t.Fatalf("index mtime changed: before=%v after=%v", beforeIndex, afterIndex)
			}
			if !beforeRepo.Equal(afterRepo) {
				t.Fatalf("repo mtime changed: before=%v after=%v", beforeRepo, afterRepo)
			}
		})
	}
}

func TestBackupGitIgnoresIgnoredFilesInEstimatesAndLFSScan(t *testing.T) {
	t.Parallel()

	repoDir, remoteDir := initCommittedRepo(t)
	configureOrigin(t, repoDir, remoteDir)
	pushMain(t, repoDir)
	writeTestFile(t, filepath.Join(repoDir, ".gitignore"), "scratch/generated/\n")
	gitRun(t, repoDir, "add", ".gitignore")
	gitRun(t, repoDir, "commit", "-m", "ignore generated")
	gitRun(t, repoDir, "push", "origin", "main")

	writeTestFile(t, filepath.Join(repoDir, "scratch", "keep.txt"), "keep-me\n")
	baseProbe := inspectBackupLocalRepository(context.Background(), repoDir, backupGitCLI{})
	if baseProbe.UntrackedBytes != int64(len("keep-me\n")) {
		t.Fatalf("base UntrackedBytes = %d, want %d", baseProbe.UntrackedBytes, len("keep-me\n"))
	}

	writeTestFile(t, filepath.Join(repoDir, "scratch", "generated", "nested", ".gitattributes"), "*.bin filter=lfs diff=lfs merge=lfs -text\n")
	writeTestFile(t, filepath.Join(repoDir, "scratch", "generated", "nested", "huge.bin"), strings.Repeat("x", 128*1024))

	probe := inspectBackupLocalRepository(context.Background(), repoDir, backupGitCLI{})
	if probe.UntrackedBytes != int64(len("keep-me\n")) {
		t.Fatalf("UntrackedBytes = %d, want %d", probe.UntrackedBytes, len("keep-me\n"))
	}
	if probe.EstimatedSourceBytes != baseProbe.EstimatedSourceBytes {
		t.Fatalf("EstimatedSourceBytes changed from %d to %d after ignored files were added", baseProbe.EstimatedSourceBytes, probe.EstimatedSourceBytes)
	}
	if probe.HasLFSAttributes {
		t.Fatalf("HasLFSAttributes = true, want false when only ignored .gitattributes contains filter=lfs")
	}
}

func TestBackupGitStateAttachedHeadUpstreamAndLocalRefs(t *testing.T) {
	t.Parallel()

	repoDir, remoteDir := initCommittedRepo(t)
	configureOrigin(t, repoDir, remoteDir)
	pushMain(t, repoDir)

	state, err := gitinspect.InspectState(context.Background(), repoDir, backupGitCLI{})
	if err != nil {
		t.Fatalf("gitinspect.InspectState() error = %v", err)
	}

	wantCommit := strings.TrimSpace(gitOutput(t, repoDir, "rev-parse", "HEAD"))
	if state.Head.Commit != wantCommit {
		t.Fatalf("Head.Commit = %q, want %q", state.Head.Commit, wantCommit)
	}
	if state.Head.Ref != "refs/heads/main" {
		t.Fatalf("Head.Ref = %q, want refs/heads/main", state.Head.Ref)
	}
	if state.Upstream == nil {
		t.Fatal("Upstream = nil, want origin/main")
	}
	if state.Upstream.Ref != "refs/remotes/origin/main" || state.Upstream.Commit != wantCommit {
		t.Fatalf("Upstream = %+v, want origin/main at %s", state.Upstream, wantCommit)
	}
	if state.LocalRefCount != 1 || !strings.HasPrefix(state.LocalRefsDigest, "sha256:") {
		t.Fatalf("local refs = %d, %q, want one ref and sha256 digest", state.LocalRefCount, state.LocalRefsDigest)
	}

	beforeDigest := state.LocalRefsDigest
	gitRun(t, repoDir, "branch", "local-copy")
	state, err = gitinspect.InspectState(context.Background(), repoDir, backupGitCLI{})
	if err != nil {
		t.Fatalf("gitinspect.InspectState() after branch error = %v", err)
	}
	if state.LocalRefCount != 2 {
		t.Fatalf("LocalRefCount = %d, want 2", state.LocalRefCount)
	}
	if state.LocalRefsDigest == beforeDigest {
		t.Fatal("LocalRefsDigest did not change after adding a local branch")
	}
}

func TestBackupGitStateDetachedAndUnbornHead(t *testing.T) {
	t.Parallel()

	t.Run("detached", func(t *testing.T) {
		repoDir, remoteDir := initCommittedRepo(t)
		configureOrigin(t, repoDir, remoteDir)
		pushMain(t, repoDir)
		gitRun(t, repoDir, "checkout", "--detach", "HEAD")

		state, err := gitinspect.InspectState(context.Background(), repoDir, backupGitCLI{})
		if err != nil {
			t.Fatalf("gitinspect.InspectState() error = %v", err)
		}
		if state.Head.Commit == "" || state.Head.Ref != "" {
			t.Fatalf("Head = %+v, want commit with no ref", state.Head)
		}
		if state.Upstream != nil {
			t.Fatalf("Upstream = %+v, want nil for detached HEAD", state.Upstream)
		}
	})

	t.Run("unborn", func(t *testing.T) {
		repoDir := t.TempDir()
		gitRun(t, repoDir, "init")

		state, err := gitinspect.InspectState(context.Background(), repoDir, backupGitCLI{})
		if err != nil {
			t.Fatalf("gitinspect.InspectState() error = %v", err)
		}
		if state.Head.Commit != "" || state.Head.Ref != "" || state.Upstream != nil {
			t.Fatalf("state = %+v, want empty HEAD and no upstream", state)
		}
		if state.LocalRefCount != 0 || state.LocalRefsDigest != "" {
			t.Fatalf("local refs = %d, %q, want empty", state.LocalRefCount, state.LocalRefsDigest)
		}
	})
}

func TestBackupGitInspectionErrorsDoNotExposeCommandOutput(t *testing.T) {
	t.Parallel()

	probe := inspectBackupLocalRepository(context.Background(), t.TempDir(), backupGitStub{
		err: &backupGitCommandError{
			Stderr:   "fatal: transport failed with synthetic credential material",
			ExitCode: 128,
			Err:      errors.New("exit status 128"),
		},
	})
	if len(probe.Errors) == 0 {
		t.Fatal("Errors = nil, want structured inspection failures")
	}
	for _, probeError := range probe.Errors {
		serialized := probeError.Code + " " + probeError.Operation + " " + probeError.Message
		if strings.Contains(serialized, "synthetic credential material") || strings.Contains(serialized, "fatal:") {
			t.Fatalf("structured error contains raw command output: %+v", probeError)
		}
		if probeError.Code == "" || probeError.Operation == "" {
			t.Fatalf("structured error lacks stable identity: %+v", probeError)
		}
	}
}

func TestBackupRemoteVerifyStates(t *testing.T) {
	t.Parallel()

	repoDir, remoteDir := initCommittedRepo(t)
	configureOrigin(t, repoDir, remoteDir)
	pushMain(t, repoDir)

	state, reason := verifyBackupRemote(context.Background(), repoDir, backupGitCLI{})
	if state != fbackup.RemoteStateReachable || reason != "" {
		t.Fatalf("verifyBackupRemote() = %q, %q", state, reason)
	}

	stub := backupGitStub{
		result: backupGitOutput{Stderr: "Repository not found"},
		err: &backupGitCommandError{
			Stderr:   "Repository not found",
			ExitCode: 128,
			Err:      errors.New("exit status 128"),
		},
	}
	state, _ = verifyBackupRemote(context.Background(), repoDir, stub)
	if state != fbackup.RemoteStateNotFound {
		t.Fatalf("not-found state = %q", state)
	}

	stub.err = &backupGitCommandError{
		Stderr:   "fatal: Authentication failed",
		ExitCode: 128,
		Err:      errors.New("exit status 128"),
	}
	state, _ = verifyBackupRemote(context.Background(), repoDir, stub)
	if state != fbackup.RemoteStateAuthError {
		t.Fatalf("auth state = %q", state)
	}

	stub.err = &backupGitCommandError{
		Stderr:   "fatal: unable to access: Could not resolve host",
		ExitCode: 128,
		Err:      errors.New("exit status 128"),
	}
	state, _ = verifyBackupRemote(context.Background(), repoDir, stub)
	if state != fbackup.RemoteStateNetworkError {
		t.Fatalf("network state = %q", state)
	}
}

func TestBackupGitRunnerSetsOptionalLocks(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "git")
	writeTestFile(t, scriptPath, "#!/bin/sh\nprintf '%s' \"$GIT_OPTIONAL_LOCKS\"\n")
	if err := os.Chmod(scriptPath, 0o755); err != nil {
		t.Fatalf("Chmod(%q) error = %v", scriptPath, err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	output, err := backupGitCLI{}.Run(context.Background(), scriptDir, "rev-parse", "--local-env-vars")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.TrimSpace(output.Stdout) != "0" {
		t.Fatalf("Run() stdout = %q, want GIT_OPTIONAL_LOCKS=0", output.Stdout)
	}
}

type backupGitStub struct {
	result backupGitOutput
	err    error
}

func (stub backupGitStub) Run(context.Context, string, ...string) (backupGitOutput, error) {
	return stub.result, stub.err
}

func initCommittedRepo(t *testing.T) (string, string) {
	t.Helper()

	baseDir := t.TempDir()
	repoDir := filepath.Join(baseDir, "repo")
	remoteDir := filepath.Join(baseDir, "remote.git")

	mustMkdirAll(t, repoDir)
	gitRun(t, repoDir, "init")
	gitRun(t, repoDir, "config", "user.name", "Backup Audit")
	gitRun(t, repoDir, "config", "user.email", "backup@example.com")
	writeTestFile(t, filepath.Join(repoDir, "tracked.txt"), "tracked\n")
	gitRun(t, repoDir, "add", "tracked.txt")
	gitRun(t, repoDir, "commit", "-m", "initial")
	gitRun(t, baseDir, "init", "--bare", remoteDir)

	return repoDir, remoteDir
}

func configureOrigin(t *testing.T, repoDir, remoteDir string) {
	t.Helper()
	gitRun(t, repoDir, "remote", "add", "origin", remoteDir)
	gitRun(t, repoDir, "branch", "-M", "main")
}

func pushMain(t *testing.T, repoDir string) {
	t.Helper()
	gitRun(t, repoDir, "push", "-u", "origin", "main")
}

func setReadOnlyTree(t *testing.T, root string) func() {
	t.Helper()

	var paths []string
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("Walk() error = %v", walkErr)
	}

	restoreModes := make(map[string]os.FileMode, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("Lstat(%q) error = %v", path, err)
		}
		restoreModes[path] = info.Mode()
		mode := os.FileMode(0o444)
		if info.IsDir() {
			mode = 0o555
		}
		if chmodErr := os.Chmod(path, mode); chmodErr != nil {
			t.Fatalf("Chmod(%q) error = %v", path, chmodErr)
		}
	}

	pathsCopy := append([]string(nil), paths...)
	return func() {
		for index := len(pathsCopy) - 1; index >= 0; index-- {
			path := pathsCopy[index]
			mode, ok := restoreModes[path]
			if !ok {
				continue
			}
			_ = os.Chmod(path, mode)
		}
	}
}

func statModTime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	return info.ModTime()
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return string(output)
}
