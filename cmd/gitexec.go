package cmd

import (
	"fmt"
	"strings"

	"github.com/zbiljic/gitexec"
)

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
