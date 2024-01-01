package cmd

import (
	"bufio"
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	pthttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/pterm/pterm"
	"github.com/tevino/abool/v2"
	giturls "github.com/whilp/git-urls"

	"github.com/zbiljic/fget/pkg/fsfind"
	"github.com/zbiljic/fget/pkg/gitexec"
	"github.com/zbiljic/fget/pkg/rhttp"
)

const (
	symrefPrefix                   = "ref: "
	errorPrefix                    = "error:"
	fatalPrefix                    = "fatal:"
	isDisabledString               = "is disabled"
	notPossibleToFastForwardString = "not possible to fast-forward"
	unableToAccessString           = "unable to access"
	butNoSuchRefWasFetchedString   = "but no such ref was fetched"
	filesWouldBeOverwrittenByMerge = "files would be overwritten by merge"
	oldMode                        = "old mode"
	newMode                        = "new mode"
	deletedFileMode                = "deleted file mode"
	couldNotReadUsername           = "could not read username"
)

var (
	ErrGitMissingRemoteHeadReference  = errors.New("missing remote head reference")
	ErrGitMissingBranchName           = errors.New("missing branch name")
	ErrGitMissingRemoteHeadBranchName = errors.New("missing remote HEAD branch name")
	ErrGitRepositoryNotReachable      = errors.New("repository not reachable")
	ErrGitRepositoryDisabled          = errors.New("repository is disabled")
	ErrGitRepositoryProtected         = errors.New("repository is protected")
)

// gitDefaultClient is used for performing requests without explicitly making
// a new client. It is purposely private to avoid modifications.
var gitDefaultClient = rhttp.NewClient(
	rhttp.WithErrorIfMovedPermanently(),
	rhttp.WithLogger(nil),
)

func gitProjectInfo(repoPath string) (string, string, *plumbing.Reference, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", "", nil, err
	}

	return gitRepoProjectInfo(repo)
}

func gitRepoProjectInfo(repo *git.Repository) (string, string, *plumbing.Reference, error) {
	remoteURL, err := gitRepoRemoteConfigURL(repo)
	if err != nil {
		return "", "", nil, err
	}

	id := filepath.Join(remoteURL.Host, remoteURL.Path)

	headRef, err := gitRepoHeadBranch(repo)
	if err != nil {
		return "", "", nil, err
	}

	return id, remoteURL.String(), headRef, nil
}

func gitRemoteURLProjectID(repoURL string) (string, error) {
	remoteURL, err := giturls.Parse(repoURL)
	if err != nil {
		return "", err
	}

	id := filepath.Join(remoteURL.Host, remoteURL.Path)

	return id, nil
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

	printProjectInfoContext(ctx)

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

func gitFixObjectNotFound(ctx context.Context, repoPath string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	_, err = worktree.Status()
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			if err1 := gitRefetch(ctx, repoPath); err1 != nil {
				return err
			}
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

	printProjectInfoContext(ctx)

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	prefixPrinter := ptermWarningWithPrefixText("reset")

	prefixPrinter.Print()

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	if err := gitReset(ctx, repoPath, plumbing.ZeroHash); err != nil {
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

func gitReset(ctx context.Context, repoPath string, commit plumbing.Hash) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	err = worktree.Reset(&git.ResetOptions{
		Commit: commit,
		Mode:   git.HardReset,
	})
	if err != nil {
		return err
	}

	return nil
}

func gitForceReset(ctx context.Context, repoPath string) error {
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

	printProjectInfoContext(ctx)

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	prefixPrinter := ptermWarningWithPrefixText("reset")

	prefixPrinter.Print()

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	if err := gitResetHead(ctx, repoPath); err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return nil
	}

	ptermSuccessMessageStyle.Println("success")

	return nil
}

func gitResetHead(ctx context.Context, repoPath string) error {
	_, err := gitRepoReset(repoPath, string(plumbing.HEAD))
	if err != nil {
		return err
	}

	return nil
}

func gitUpdateDefaultBranch(ctx context.Context, repoPath string) error {
	_, remoteURL, headRef, err := gitProjectInfo(repoPath)
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

	// NOTE: delayed error check
	if err != nil {
		printProjectInfoContext(ctx)

		if errors.Is(err, ErrGitMissingRemoteHeadReference) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return err
		}

		if errors.Is(err, ErrGitRepositoryNotReachable) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return nil
		}

		if errors.Is(err, ErrGitRepositoryDisabled) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return err
		}

		if errors.Is(err, ErrGitRepositoryProtected) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return err
		}

		if errors.Is(err, rhttp.ErrHttpMovedPermanently) {
			var urlError *url.Error
			if errors.As(err, &urlError) {
				prefixPrinter.Printfln("moved: %s", urlError.URL)
				return &GitRepositoryMovedError{OldURL: remoteURL, NewURL: urlError.URL}
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

	printProjectInfoContext(ctx)

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

func gitResetDefaultBranch(ctx context.Context, repoPath string) error {
	_, remoteURL, headRef, err := gitProjectInfo(repoPath)
	if err != nil {
		return err
	}

	prefixPrinter := ptermWarningWithPrefixText("reset HEAD")

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

	// NOTE: delayed error check
	if err != nil {
		printProjectInfoContext(ctx)

		if errors.Is(err, ErrGitMissingRemoteHeadReference) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return nil
		}

		if errors.Is(err, ErrGitRepositoryNotReachable) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return nil
		}

		if errors.Is(err, ErrGitRepositoryDisabled) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return err
		}

		if errors.Is(err, ErrGitRepositoryProtected) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return err
		}

		if errors.Is(err, rhttp.ErrHttpMovedPermanently) {
			var urlError *url.Error
			if errors.As(err, &urlError) {
				prefixPrinter.Printfln("moved: %s", urlError.URL)
				return &GitRepositoryMovedError{OldURL: remoteURL, NewURL: urlError.URL}
			} else {
				prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			}
			return nil
		}

		// NOTE: ignore all errors here
		prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
		return nil
	}

	if headRef.Hash() == remoteHeadRef.Hash() {
		return nil
	}

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	printProjectInfoContext(ctx)

	prefixPrinter.Printf("'%s'", headRef.Name().Short())
	pterm.Print(": ")

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	if err := gitReset(ctx, repoPath, remoteHeadRef.Hash()); err != nil {
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

	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodHead, remoteURL.String(), nil)
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
		Symref: true,
	})
	if err != nil {
		outString := string(out)
		outString = strings.ToLower(outString)
		// check if repository is disabled
		if strings.HasPrefix(outString, errorPrefix) && strings.Contains(outString, isDisabledString) {
			return nil, ErrGitRepositoryDisabled
		}
		// check if auth required
		if strings.HasPrefix(outString, fatalPrefix) && strings.Contains(outString, couldNotReadUsername) {
			return nil, ErrGitRepositoryProtected
		}

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

func gitCheckAndPull(ctx context.Context, repoPath string) error {
	if isRemoteUpToDate, err := gitIsRemoteUpToDate(ctx, repoPath); err != nil {
		return err
	} else if isRemoteUpToDate {
		return git.NoErrAlreadyUpToDate
	}

	pterm.EnableDebugMessages()
	defer pterm.DisableDebugMessages()

	prefixPrinter := ptermDebugWithPrefixText("pull")

	exp := backoff.NewExponentialBackOff()
	exp.InitialInterval = retryWaitMin
	exp.MaxInterval = retryWaitMax
	exp.MaxElapsedTime = retryMaxElapsedTime

	var attempt int
	var err error

	for i := 0; ; i++ {
		if attempt >= retryMaxAttempts {
			break
		}

		attempt++

		if attempt > 1 {
			prefixPrinter.Printfln("attempt: %d", attempt)
		}

		// attempt the pull
		err = gitPull(ctx, repoPath)

		switch {
		case err == nil:
			return nil
		case errors.Is(err, transport.ErrAuthenticationRequired):
			return nil
		case errors.Is(err, plumbing.ErrObjectNotFound):
			if err1 := gitRefetch(ctx, repoPath); err1 != nil {
				return err
			}
			// retry
		case errors.Is(err, git.ErrWorktreeNotClean):
			if err1 := gitIgnoreFileMode(ctx, repoPath); err1 != nil {
				return err
			}
			if err1 := gitForceReset(ctx, repoPath); err1 != nil {
				return err
			}
			// retry
		case errors.Is(err, git.ErrNonFastForwardUpdate):
			if err1 := gitResetDefaultBranch(ctx, repoPath); err1 != nil {
				return err
			}
			// retry
		case errors.Is(err, git.ErrUnstagedChanges):
			if err1 := gitMakeClean(ctx, repoPath); err1 != nil {
				return err
			}
			// retry
		case errors.Is(err, plumbing.ErrReferenceNotFound):
			if err1 := gitUpdateDefaultBranch(ctx, repoPath); err1 != nil {
				return err1
			}
			// NOTE: skip backoff, fast retry
			continue
		case errors.Is(err, storage.ErrReferenceHasChanged):
			if err1 := gitFixReferences(ctx, repoPath); err1 != nil {
				return err
			}
			// NOTE: skip backoff, fast retry
			continue
		case errors.Is(err, git.NoErrAlreadyUpToDate):
			return err
		case errors.Is(err, ErrGitRepositoryNotReachable):
			return err
		default:
			switch v := err.(type) {
			case *url.Error:
				// Don't retry if the error was due to TLS cert verification failure.
				if _, ok := v.Err.(x509.HostnameError); ok {
					return nil
				}
				return err
			case *plumbing.UnexpectedError:
				if pthttpErr, ok := v.Err.(*pthttp.Err); ok {
					// don't retry on server errors
					if pthttpErr.Response.StatusCode >= http.StatusInternalServerError {
						return nil
					}
				}
				return err
			case *exec.ExitError:
				switch v.ExitCode() {
				case 1:
					if err1 := gitMakeClean(ctx, repoPath); err1 != nil {
						return err
					}
					// retry
				case 128:
					if err1 := gitResetDefaultBranch(ctx, repoPath); err1 != nil {
						return err
					}
					// retry
				}
			default:
			}
		}

		if attempt == 1 {
			// reset for first retry
			exp.Reset()
		}

		wait := exp.NextBackOff()
		if wait == backoff.Stop {
			break
		}

		remain := retryMaxElapsedTime - exp.GetElapsedTime()

		prefixPrinter.Printfln("retrying in %s (%s left)", wait.Round(time.Millisecond), remain.Round(time.Second))

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	if err != nil {
		prefixPrinter.Printfln("giving up after %d attempt(s): %s", attempt, err.Error())
	}

	return err
}

func gitIsRemoteUpToDate(ctx context.Context, repoPath string) (bool, error) {
	_, remoteURL, headRef, err := gitProjectInfo(repoPath)
	if err != nil {
		return false, err
	}

	prefixPrinter := ptermDescriptionWithPrefixText("remote")

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

	// NOTE: delayed error check
	if err != nil {
		printProjectInfoContext(ctx)

		if errors.Is(err, ErrGitMissingRemoteHeadReference) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return false, nil
		}

		if errors.Is(err, ErrGitRepositoryNotReachable) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return false, err
		}

		if errors.Is(err, ErrGitRepositoryDisabled) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return false, err
		}

		if errors.Is(err, ErrGitRepositoryProtected) {
			prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			return false, err
		}

		if errors.Is(err, rhttp.ErrHttpMovedPermanently) {
			var urlError *url.Error
			if errors.As(err, &urlError) {
				prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.WarningMessageStyle).Printfln("moved: %s", urlError.URL)
				return false, &GitRepositoryMovedError{OldURL: remoteURL, NewURL: urlError.URL}
			} else {
				prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
			}
			return false, nil
		}

		// NOTE: ignore all errors here
		prefixPrinter.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).Println(err.Error())
		return false, nil
	}

	onlyUpdated, _ := ctx.Value(ctxKeyOnlyUpdated{}).(bool)

	if headRef.Hash() == remoteHeadRef.Hash() {
		if !onlyUpdated {
			printProjectInfoContext(ctx)
			prefixPrinter.Println("up-to-date")
		}

		return true, nil
	}

	return false, nil
}

func gitPull(ctx context.Context, repoPath string) error {
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

	printProjectInfoContext(ctx)

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	prefixPrinter := ptermInfoWithPrefixText("pull")

	prefixPrinter.Print()

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	out, err := gitRepoPathPull(repoPath)
	if err != nil {
		if len(out) > 0 {
			outString := string(out)
			outString = strings.ToLower(outString)
			// check if not possible to fast forward
			if strings.HasPrefix(outString, fatalPrefix) && strings.Contains(outString, notPossibleToFastForwardString) {
				err = git.ErrNonFastForwardUpdate
			}
			// check if accessible
			if strings.HasPrefix(outString, fatalPrefix) && strings.Contains(outString, unableToAccessString) {
				err = ErrGitRepositoryNotReachable
			}
			// check if there are local changes
			if strings.HasPrefix(outString, errorPrefix) && strings.Contains(outString, filesWouldBeOverwrittenByMerge) {
				err = git.ErrWorktreeNotClean
			}
			// check if default branch is changed
			if strings.Contains(outString, butNoSuchRefWasFetchedString) {
				err = plumbing.ErrReferenceNotFound
			}
		}

		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	ptermSuccessMessageStyle.Println("success")

	if len(out) > 0 {
		pterm.Println()
		pterm.Println(string(out))
	}

	return nil
}

func gitPackObjectsCount(ctx context.Context, repoPath string) (int, error) {
	objects, err := fsfind.GitObjects(repoPath)
	if err != nil {
		return 0, fmt.Errorf("objects count: %w", err)
	}

	return len(objects), nil
}

func gitGc(ctx context.Context, repoPath string) error {
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

	printProjectInfoContext(ctx)

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	prefixPrinter := ptermInfoWithPrefixText("gc")

	prefixPrinter.Print()

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	out, err := gitRepoPathGc(repoPath)
	if err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	ptermSuccessMessageStyle.Println("success")

	if len(out) > 0 {
		pterm.Println()
		pterm.Println(string(out))
	}

	return nil
}

func gitRefetch(ctx context.Context, repoPath string) error {
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

	printProjectInfoContext(ctx)

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	prefixPrinter := ptermInfoWithPrefixText("fetch --refetch")

	prefixPrinter.Print()

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	out, err := gitRepoRefetch(repoPath)
	if err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	ptermSuccessMessageStyle.Println("success")

	if len(out) > 0 {
		pterm.Println()
		pterm.Println(string(out))
	}

	return nil
}

func gitIgnoreFileMode(ctx context.Context, repoPath string) error {
	isDiffFileMode, err := gitIsDiffFileMode(ctx, repoPath)
	if err != nil {
		return err
	}

	if isDiffFileMode {
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

	printProjectInfoContext(ctx)

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	prefixPrinter := ptermInfoWithPrefixText("config core.filemode false")

	prefixPrinter.Print()

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	out, err := gitRepoIgnoreFileMode(repoPath)
	if err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	ptermSuccessMessageStyle.Println("success")

	if len(out) > 0 {
		pterm.Println()
		pterm.Println(string(out))
	}

	return nil
}

func gitIsDiffFileMode(ctx context.Context, repoPath string) (bool, error) {
	out, err := gitRepoDiff(repoPath)
	if err != nil {
		return false, err
	}

	if len(out) == 0 {
		return false, nil
	}

	outString := string(out)
	outString = strings.ToLower(outString)
	// check if diff due to file mode
	if strings.Contains(outString, oldMode) ||
		strings.Contains(outString, newMode) ||
		strings.Contains(outString, deletedFileMode) {
		return true, nil
	}

	return false, nil
}

func gitMove(ctx context.Context, repoPath, oldURL, newURL string) error {
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

	printProjectInfoContext(ctx)

	dryRun, _ := ctx.Value(ctxKeyDryRun{}).(bool)

	prefixPrinter := ptermInfoWithPrefixText("moving")

	prefixPrinter.Printf("from '%s' to '%s'", oldURL, newURL)

	pterm.Println()

	prefixPrinter.Print()

	if dryRun {
		ptermSuccessMessageStyle.Println("dry-run")
		return nil
	}

	oldID, err := gitRemoteURLProjectID(oldURL)
	if err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	newID, err := gitRemoteURLProjectID(newURL)
	if err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	if !strings.HasSuffix(repoPath, oldID) {
		err = fmt.Errorf("unexpected repository path: %s", repoPath)
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	commonRepoPath := strings.TrimSuffix(repoPath, oldID)

	fs := osfs.New(commonRepoPath)

	newRepoPath := filepath.Join(commonRepoPath, newID)

	// check if destination exists
	if _, err := fs.Stat(newID); err != nil {
		if os.IsExist(err) {
			ptermErrorMessageStyle.Println(err.Error())
			return err
		}
	} else {
		ptermWarningMessageStyle.Printfln("already exists: %s", newRepoPath)

		// just remove from old repo path
		err = os.RemoveAll(repoPath)
		if err != nil {
			return err
		}

		return nil
	}

	err = fs.MkdirAll(filepath.Dir(newID), os.ModePerm)
	if err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	err = fs.Rename(oldID, newID)
	if err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	newRepo, err := git.PlainOpen(newRepoPath)
	if err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	config, err := newRepo.Config()
	if err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	remote, ok := config.Remotes[git.DefaultRemoteName]
	if !ok {
		err = fmt.Errorf("missing remote: %s", git.DefaultRemoteName)
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	if repoURL := remote.URLs[0]; repoURL != "" {
		// update to new URL
		remote.URLs[0] = newURL
	}

	// save updated config
	err = newRepo.SetConfig(config)
	if err != nil {
		ptermErrorMessageStyle.Println(err.Error())
		return err
	}

	ptermSuccessMessageStyle.Println("success")

	return nil
}
