package cmd

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pterm/pterm"
	giturls "github.com/whilp/git-urls"
)

func gitProjectInfo(repoPath string) (string, string, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", "", err
	}

	return gitRepoProjectInfo(repo)
}

func gitRepoProjectInfo(repo *git.Repository) (string, string, error) {
	var (
		id         string
		branchName string
	)

	remote, err := repo.Remote(git.DefaultRemoteName)
	if err != nil {
		return "", "", err
	}

	if repoURL := remote.Config().URLs[0]; repoURL != "" {
		// parse URI
		parsedURI, err := giturls.Parse(repoURL)
		if err != nil {
			return "", "", err
		}

		id = filepath.Join(parsedURI.Host, parsedURI.Path)
	} else {
		return "", "", errors.New("empty repository remote URL")
	}

	branchName, err = gitRepoHeadBranch(repo)
	if err != nil {
		return "", "", err
	}

	return id, branchName, nil
}

func gitRepoHeadBranch(repo *git.Repository) (string, error) {
	headRef, err := repo.Head()
	if err != nil {
		return "", err
	}

	config, err := repo.Config()
	if err != nil {
		return "", err
	}

	for branchName, branchConfig := range config.Branches {
		if headRef.Name() == branchConfig.Merge {
			return branchName, nil
		}
	}

	return "", errors.New("repository branch name")
}

func gitFixReferences(ctx context.Context, repoPath string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	it, err := repo.Storer.IterReferences()
	if err != nil {
		return err
	}
	defer it.Close()

	err = it.ForEach(func(ref *plumbing.Reference) error {
		// exit this iteration early for non-hash references
		if ref.Type() != plumbing.HashReference {
			return nil
		}

		// skip tags
		if ref.Name().IsTag() {
			return nil
		}

		if ref.Hash().IsZero() {
			if err := gitRemoveReference(ctx, repoPath, ref.Name()); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func gitRemoveReference(ctx context.Context, repoPath string, refName plumbing.ReferenceName) error {
	updateMutex.Lock()
	defer updateMutex.Unlock()

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	project, _, err := gitRepoProjectInfo(repo)
	if err != nil {
		return err
	}

	pterm.Print(project)
	pterm.Print(" [")
	pterm.NewStyle(pterm.ThemeDefault.WarningMessageStyle...).Printf("'%s': reference broken", refName)
	pterm.Print("]: ")

	if dryRun {
		pterm.NewStyle(pterm.ThemeDefault.SuccessMessageStyle...).Println("dry-run")
	} else {
		err := repo.Storer.RemoveReference(refName)
		if err != nil {
			pterm.NewStyle(pterm.ThemeDefault.ErrorMessageStyle...).Println(err.Error())
		} else {
			pterm.NewStyle(pterm.ThemeDefault.SuccessMessageStyle...).Println("success")
		}
	}

	return nil
}

func gitMakeClean(ctx context.Context, repoPath string) error {
	isClean, err := gitIsClean(ctx, repoPath)
	if err != nil {
		return err
	}

	if isClean {
		return nil
	}

	updateMutex.Lock()
	defer updateMutex.Unlock()

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	project, _, err := gitProjectInfo(repoPath)
	if err != nil {
		return err
	}

	pterm.Print(project)
	pterm.Print(" [")
	pterm.NewStyle(pterm.ThemeDefault.WarningMessageStyle...).Print("reset")
	pterm.Print("]: ")

	if dryRun {
		pterm.NewStyle(pterm.ThemeDefault.SuccessMessageStyle...).Println("dry-run")
	} else {
		err = gitReset(ctx, repoPath)
		if err != nil {
			pterm.NewStyle(pterm.ThemeDefault.ErrorMessageStyle...).Println(err.Error())
		} else {
			pterm.NewStyle(pterm.ThemeDefault.SuccessMessageStyle...).Println("success")
		}
	}

	return nil
}

func gitIsClean(ctx context.Context, repoPath string) (bool, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return false, err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return false, err
	}

	status, err := worktree.Status()
	if err != nil {
		return false, err
	}

	return status.IsClean(), nil
}

func gitReset(ctx context.Context, repoPath string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	err = worktree.Reset(&git.ResetOptions{
		Mode: git.HardReset,
	})
	if err != nil {
		return err
	}

	return nil
}
