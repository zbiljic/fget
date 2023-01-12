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

func gitProjectID(repoPath string) (string, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", err
	}

	return gitRepoProjectID(repo)
}

func gitRepoProjectID(repo *git.Repository) (string, error) {
	remote, err := repo.Remote(git.DefaultRemoteName)
	if err != nil {
		return "", err
	}

	if repoURL := remote.Config().URLs[0]; repoURL != "" {
		// parse URI
		parsedURI, err := giturls.Parse(repoURL)
		if err != nil {
			return "", err
		}

		project := filepath.Join(parsedURI.Host, parsedURI.Path)

		return project, nil
	}

	return "", errors.New("failed to read repo URL")
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

	project, err := gitRepoProjectID(repo)
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

	project, err := gitProjectID(repoPath)
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
