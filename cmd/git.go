package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pterm/pterm"
	giturls "github.com/whilp/git-urls"
	"github.com/zbiljic/fget/pkg/gitexec"
)

const symrefPrefix = "ref: "

const httpGitRemoteCheckTimeout = 10 * time.Second

var (
	ErrGitMissingRemoteHeadReference  = errors.New("missing remote head reference")
	ErrGitMissingBranchName           = errors.New("missing branch name")
	ErrGitMissingRemoteHeadBranchName = errors.New("missing remote HEAD branch name")
	ErrGitRepositoryNotReachable      = errors.New("repository not reachable")
)

var ErrHttpMovedPermanently = errors.New("moved permanently")

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

func gitRemoteConfigURL(repoPath string) (*url.URL, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}

	return gitRepoRemoteConfigURL(repo)
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

func gitUpdateDefaultBranch(ctx context.Context, repoPath string) error {
	_, headRef, err := gitProjectInfo(repoPath)
	if err != nil {
		return err
	}

	prefixPrinter := pterm.Warning.WithPrefix(pterm.Prefix{
		Style: pterm.Warning.Prefix.Style,
		Text:  "update HEAD",
	})

	remoteHeadRef, err := gitFindRemoteHeadReference(ctx, repoPath)
	if err != nil {
		if errors.Is(err, ErrGitMissingRemoteHeadReference) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return nil
		}

		if errors.Is(err, ErrGitRepositoryNotReachable) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return nil
		}

		if errors.Is(err, ErrHttpMovedPermanently) {
			var urlError *url.Error
			if errors.As(err, &urlError) {
				prefixPrinter.Printf("moved: %s\n", urlError.URL)
			} else {
				prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			}
			return nil
		}

		// NOTE: ignore all errors here
		prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
		return nil
	}

	branchName := headRef.Name().Short()
	remoteHeadBranchName := remoteHeadRef.Name().Short()

	if branchName == "" {
		return ErrGitMissingBranchName
	}

	if remoteHeadBranchName == "" {
		return ErrGitMissingRemoteHeadBranchName
	}

	if branchName == remoteHeadBranchName {
		return nil
	}

	updateMutex.Lock()
	defer updateMutex.Unlock()

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	prefixPrinter.Println(remoteHeadBranchName)

	if dryRun {
		prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.SuccessMessageStyle).Println("dry-run")
	} else {
		err = gitReplaceDefaultBranch(ctx, repoPath, headRef, remoteHeadRef)
		if err != nil {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
		} else {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.SuccessMessageStyle).Println("success")
		}
	}

	return nil
}

func gitCheckRemoteURL(ctx context.Context, repoPath string) (bool, error) {
	remoteURL, err := gitRemoteConfigURL(repoPath)
	if err != nil {
		return false, err
	}

	httpClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// default behavior
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}

			if req.Response.StatusCode == http.StatusMovedPermanently {
				return ErrHttpMovedPermanently
			}

			return nil
		},
	}

	ctx, cancel := context.WithTimeout(ctx, httpGitRemoteCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL.String(), nil)
	if err != nil {
		return false, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}

	ok := resp.StatusCode == http.StatusOK

	return ok, nil
}

func gitFindRemoteHeadReference(ctx context.Context, repoPath string) (*plumbing.Reference, error) {
	ok, err := gitCheckRemoteURL(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, ErrGitRepositoryNotReachable
	}

	out, err := gitexec.LsRemote(&gitexec.LsRemoteOptions{
		CmdDir: repoPath,
		Quiet:  true,
		Symref: true,
	})
	if err != nil {
		return nil, err
	}

	var ref *plumbing.Reference

	buf := bytes.NewBuffer(out)
	scanner := bufio.NewScanner(buf)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, symrefPrefix) && strings.Contains(line, string(plumbing.HEAD)) {
			split := strings.Split(line, "\t")
			ref = plumbing.NewReferenceFromStrings(split[1], split[0])
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading standard input: %w", err)
	}

	if ref == nil {
		return nil, ErrGitMissingRemoteHeadReference
	}

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}

	remote, err := repo.Remote(git.DefaultRemoteName)
	if err != nil {
		return nil, err
	}

	refs, err := remote.List(&git.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, r := range refs {
		if r.Name() == ref.Target() {
			ref = plumbing.NewHashReference(ref.Target(), r.Hash())
			break
		}
	}

	return ref, nil
}

func gitReplaceDefaultBranch(ctx context.Context, repoPath string, from, to *plumbing.Reference) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	err = repo.CreateBranch(&config.Branch{
		Name:   to.Name().Short(),
		Remote: git.DefaultRemoteName,
		Merge:  to.Name(),
	})
	if err != nil {
		if !errors.Is(err, git.ErrBranchExists) {
			return err
		}
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	checkoutOpts := &git.CheckoutOptions{
		Hash:   to.Hash(),
		Branch: to.Name(),
		Create: true,
		Force:  true,
	}

	_, err = repo.Storer.Reference(to.Name())
	if err == nil {
		checkoutOpts.Hash = plumbing.ZeroHash
		checkoutOpts.Create = false
	}

	err = worktree.Checkout(checkoutOpts)
	if err != nil {
		return err
	}

	err = repo.Storer.RemoveReference(from.Name())
	if err != nil {
		return err
	}

	err = repo.DeleteBranch(from.Name().Short())
	if err != nil {
		return err
	}

	return nil
}
