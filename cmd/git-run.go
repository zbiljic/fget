package cmd

import "context"

func gitRunFix(ctx context.Context, repoPath string) error {
	if err := gitFixReferences(ctx, repoPath); err != nil {
		return err
	}

	if err := gitMakeClean(ctx, repoPath); err != nil {
		return err
	}

	if err := gitUpdateDefaultBranch(ctx, repoPath); err != nil {
		return err
	}

	return nil
}
