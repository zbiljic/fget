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
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil
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
