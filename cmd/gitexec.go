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
