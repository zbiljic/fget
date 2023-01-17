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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/pterm/pterm"
	"github.com/tevino/abool/v2"
	giturls "github.com/whilp/git-urls"

	"github.com/zbiljic/fget/pkg/gitexec"
	"github.com/zbiljic/fget/pkg/rhttp"
)

const symrefPrefix = "ref: "

var (
	ErrGitMissingRemoteHeadReference  = errors.New("missing remote head reference")
	ErrGitMissingBranchName           = errors.New("missing branch name")
	ErrGitMissingRemoteHeadBranchName = errors.New("missing remote HEAD branch name")
	ErrGitRepositoryNotReachable      = errors.New("repository not reachable")
)

// gitDefaultClient is used for performing requests without explicitly making
// a new client. It is purposely private to avoid modifications.
var gitDefaultClient = rhttp.NewClient(
	rhttp.WithErrorIfMovedPermanently(),
	rhttp.WithLogger(nil),
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
	}

	return nil, errors.New("empty repository remote URL")
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
	// complicated update locking
	if isUpdateMutexLocked, ok := ctx.Value(ctxKeyIsUpdateMutexLocked{}).(*abool.AtomicBool); ok {
		if isUpdateMutexLocked.IsNotSet() {
			updateMutex.Lock()
			isUpdateMutexLocked.Set()
		}
	} else {
		// simple
		updateMutex.Lock()
	}
	if shouldUpdateMutexUnlock, ok := ctx.Value(ctxKeyShouldUpdateMutexUnlock{}).(bool); ok {
		if shouldUpdateMutexUnlock {
			defer updateMutex.Unlock()
		}
	} else {
		// simple
		defer updateMutex.Unlock()
	}

	// print info if executing
	if printProjectInfoHeaderFn, ok := ctx.Value(ctxKeyPrintProjectInfoHeaderFn{}).(func()); ok {
		printProjectInfoHeaderFn()
	}

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	prefixPrinter := ptermWarningWithPrefixText("remove reference")

	prefixPrinter.Printf("'%s'", refName)
	pterm.Print(": ")

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	if err := repo.Storer.RemoveReference(refName); err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return nil
	}

	ptermSuccessMessageStyle.Println("success")

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

	// complicated update locking
	if isUpdateMutexLocked, ok := ctx.Value(ctxKeyIsUpdateMutexLocked{}).(*abool.AtomicBool); ok {
		if isUpdateMutexLocked.IsNotSet() {
			updateMutex.Lock()
			isUpdateMutexLocked.Set()
		}
	} else {
		// simple
		updateMutex.Lock()
	}
	if shouldUpdateMutexUnlock, ok := ctx.Value(ctxKeyShouldUpdateMutexUnlock{}).(bool); ok {
		if shouldUpdateMutexUnlock {
			defer updateMutex.Unlock()
		}
	} else {
		// simple
		defer updateMutex.Unlock()
	}

	// print info if executing
	if printProjectInfoHeaderFn, ok := ctx.Value(ctxKeyPrintProjectInfoHeaderFn{}).(func()); ok {
		printProjectInfoHeaderFn()
	}

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	prefixPrinter := ptermWarningWithPrefixText("reset")

	prefixPrinter.Print()

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	if err := gitReset(ctx, repoPath); err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return nil
	}

	ptermSuccessMessageStyle.Println("success")

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

	prefixPrinter := ptermWarningWithPrefixText("update HEAD")

	remoteHeadRef, err := gitFindRemoteHeadReference(ctx, repoPath)
	// NOTE: error check comes after lock

	// complicated update locking
	if isUpdateMutexLocked, ok := ctx.Value(ctxKeyIsUpdateMutexLocked{}).(*abool.AtomicBool); ok {
		if isUpdateMutexLocked.IsNotSet() {
			updateMutex.Lock()
			isUpdateMutexLocked.Set()
		}
	} else {
		// simple
		updateMutex.Lock()
	}
	if shouldUpdateMutexUnlock, ok := ctx.Value(ctxKeyShouldUpdateMutexUnlock{}).(bool); ok {
		if shouldUpdateMutexUnlock {
			defer updateMutex.Unlock()
		}
	} else {
		// simple
		defer updateMutex.Unlock()
	}

	// print info if executing
	if printProjectInfoHeaderFn, ok := ctx.Value(ctxKeyPrintProjectInfoHeaderFn{}).(func()); ok {
		printProjectInfoHeaderFn()
	}

	// NOTE: delayed error check
	if err != nil {
		if errors.Is(err, ErrGitMissingRemoteHeadReference) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return nil
		}

		if errors.Is(err, ErrGitRepositoryNotReachable) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return nil
		}

		if errors.Is(err, rhttp.ErrHttpMovedPermanently) {
			var urlError *url.Error
			if errors.As(err, &urlError) {
				prefixPrinter.Printfln("moved: %s", urlError.URL)
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

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	prefixPrinter.Printf("'%s'", remoteHeadBranchName)
	pterm.Print(": ")

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	if err := gitReplaceDefaultBranch(ctx, repoPath, headRef, remoteHeadRef); err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return nil
	}

	ptermSuccessMessageStyle.Println("success")

	return nil
}

func gitCheckRemoteURL(ctx context.Context, repoPath string) (bool, error) {
	remoteURL, err := gitRemoteConfigURL(repoPath)
	if err != nil {
		return false, err
	}

	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, remoteURL.String(), nil)
	if err != nil {
		return false, err
	}

	resp, err := gitDefaultClient.Do(req)
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
