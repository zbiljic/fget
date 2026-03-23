package cmd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/zbiljic/gitexec"
)

func gitCommandOutputContext(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)

	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(exitErr.Stderr)))
		}

		return out, err
	}

	return out, nil
}

func gitRepoPathPull(repoPath string) ([]byte, error) {
	out, err := gitexec.Pull(&gitexec.PullOptions{
		CmdDir: repoPath,
		Prune:  true,
	})
	if err != nil {
		return out, err
	}

	return out, nil
}

func gitRepoFetch(repoPath string) ([]byte, error) {
	out, err := gitexec.Fetch(&gitexec.FetchOptions{
		CmdDir: repoPath,
		Prune:  true,
	})
	if err != nil {
		return out, err
	}

	return out, nil
}

func gitRepoRefetch(repoPath string) ([]byte, error) {
	out, err := gitexec.Fetch(&gitexec.FetchOptions{
		CmdDir:  repoPath,
		Prune:   true,
		Refetch: true,
	})
	if err != nil {
		return out, err
	}

	return out, nil
}

func gitRepoPathGc(repoPath string) ([]byte, error) {
	out, err := gitexec.Gc(&gitexec.GcOptions{
		CmdDir: repoPath,
		Prune:  "all",
	})
	if err != nil {
		return out, err
	}

	return out, nil
}

func gitRepoReset(repoPath, commit string) ([]byte, error) {
	out, err := gitexec.Reset(&gitexec.ResetOptions{
		CmdDir: repoPath,
		Hard:   true,
		Commit: commit,
	})
	if err != nil {
		return out, err
	}

	return out, nil
}

func gitRepoDiff(repoPath string) ([]byte, error) {
	out, err := gitexec.Diff(&gitexec.DiffOptions{
		CmdDir: repoPath,
		Cached: true,
	})
	if err != nil {
		return out, err
	}

	return out, nil
}

func gitRepoIgnoreFileMode(repoPath string) ([]byte, error) {
	out, err := gitexec.Command(repoPath, "config", "core.filemode", "false")
	if err != nil {
		return out, err
	}

	return out, nil
}

func gitRepoCommitCount(repoPath string) (int, error) {
	out, err := gitexec.RevList(&gitexec.RevListOptions{
		CmdDir: repoPath,
		Count:  true,
		Commit: "HEAD",
	})
	if err != nil {
		return 0, err
	}

	count := 0
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func gitRepoCommitCountContext(ctx context.Context, repoPath string) (int, error) {
	out, err := gitCommandOutputContext(ctx, repoPath, "rev-list", "--count", "HEAD")
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, err
	}

	return count, nil
}

func gitRepoIsCleanContext(ctx context.Context, repoPath string) (bool, error) {
	out, err := gitCommandOutputContext(ctx, repoPath, "status", "--porcelain=v1")
	if err != nil {
		return false, err
	}

	return len(strings.TrimSpace(string(out))) == 0, nil
}

func gitLastCommitDateContext(ctx context.Context, repoPath string) (time.Time, error) {
	out, err := gitCommandOutputContext(ctx, repoPath, "log", "-1", "--format=%cI", "HEAD")
	if err != nil {
		return time.Time{}, err
	}

	commitDate, err := time.Parse(time.RFC3339, strings.TrimSpace(string(out)))
	if err != nil {
		return time.Time{}, err
	}

	return commitDate, nil
}

func gitRepoLsRemote(repoPath string) ([]byte, error) {
	out, err := gitexec.LsRemote(&gitexec.LsRemoteOptions{
		CmdDir: repoPath,
		Symref: true,
	})
	if err != nil {
		return out, err
	}

	return out, nil
}
