package cmd

import (
	"context"
	"errors"

	"github.com/go-git/go-git/v5"
)

func gitRunFix(ctx context.Context, repoPath string) error {
	if err := gitFixReferences(ctx, repoPath); err != nil {
		return err
	}

	if err := gitFixObjectNotFound(ctx, repoPath); err != nil {
		return err
	}

	if err := gitMakeClean(ctx, repoPath); err != nil {
		return err
	}

	if err := gitUpdateDefaultBranch(ctx, repoPath); err != nil {
		return err
	}

	if err := gitResetDefaultBranch(ctx, repoPath); err != nil {
		return err
	}

	return nil
}

func gitRunUpdate(ctx context.Context, repoPath string) error {
	if err := gitCheckAndPull(ctx, repoPath); err != nil {
		switch {
		case errors.Is(err, git.NoErrAlreadyUpToDate):
			fallthrough
		case errors.Is(err, ErrGitMissingRemoteHeadReference):
			fallthrough
		case errors.Is(err, ErrGitRepositoryNotReachable):
			fallthrough
		case errors.Is(err, ErrGitRepositoryDisabled):
			return nil
		case errors.Is(err, ErrGitRepositoryProtected):
			return nil
		default:
			//nolint:gocritic
			switch v := err.(type) {
			case *GitRepositoryMovedError:
				if err1 := gitMove(ctx, repoPath, v.OldURL, v.NewURL); err1 != nil {
					return err
				}
				return nil
			}
		}

		return err
	}

	if err := gitMakeClean(ctx, repoPath); err != nil {
		return err
	}

	if err := gitRunGc(ctx, repoPath); err != nil {
		return err
	}

	return nil
}

func gitRunGc(ctx context.Context, repoPath string) error {
	if count, err := gitPackObjectsCount(ctx, repoPath); err != nil {
		return err
	} else if count <= 1 {
		return nil
	}

	if err := gitGc(ctx, repoPath); err != nil {
		return err
	}

	return nil
}

func gitRunReclone(ctx context.Context, repoPath string) error {
	if err := gitReclone(ctx, repoPath); err != nil {
		return err
	}

	return nil
}
