package cmd

import "github.com/zbiljic/fget/pkg/gitexec"

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
	out, err := gitexec.Config(&gitexec.ConfigOptions{
		CmdDir: repoPath,
		Name:   "core.filemode",
		Value:  "false",
	})
	if err != nil {
		return out, err
	}

	return out, nil
}
