package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pterm/pterm"
	giturls "github.com/whilp/git-urls"
)

func gitProjectInfo(repoPath string) (string, *plumbing.Reference, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", nil, err
	}

	return gitRepoProjectInfo(repo)
}

func gitRepoProjectInfo(repo *git.Repository) (string, *plumbing.Reference, error) {
	remoteURL, err := gitRepoRemoteConfigURL(repo)
	if err != nil {
		return "", nil, err
	}

	id := filepath.Join(remoteURL.Host, remoteURL.Path)

	headRef, err := gitRepoHeadBranch(repo)
	if err != nil {
		return "", nil, err
	}

	return id, headRef, nil
}

func gitRepoRemoteConfigURL(repo *git.Repository) (*url.URL, error) {
	remote, err := repo.Remote(git.DefaultRemoteName)
	if err != nil {
		return nil, err
	}

	if repoURL := remote.Config().URLs[0]; repoURL != "" {
		// parse URI
		parsedURI, err := giturls.Parse(repoURL)
		if err != nil {
			return nil, err
		}

		return parsedURI, nil
	} else {
		return nil, errors.New("empty repository remote URL")
	}
}

func gitRepoHeadBranch(repo *git.Repository) (*plumbing.Reference, error) {
	headRef, err := repo.Head()
	if err != nil {
		return nil, err
	}

	return headRef, nil
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

	prefixPrinter := pterm.Warning.WithPrefix(pterm.Prefix{
		Style: pterm.Warning.Prefix.Style,
		Text:  fmt.Sprintf("reference broken: '%s'", refName),
	})

	if dryRun {
		prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.SuccessMessageStyle).Println("dry-run")
	} else {
		err := repo.Storer.RemoveReference(refName)
		if err != nil {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
		} else {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.SuccessMessageStyle).Println("success")
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

	prefixPrinter := pterm.Warning.WithPrefix(pterm.Prefix{
		Style: pterm.Warning.Prefix.Style,
		Text:  "reset",
	})

	if dryRun {
		prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.SuccessMessageStyle).Println("dry-run")
	} else {
		err = gitReset(ctx, repoPath)
		if err != nil {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
		} else {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.SuccessMessageStyle).Println("success")
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
