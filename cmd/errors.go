package cmd

import "fmt"

type GitRepositoryMovedError struct {
	OldURL string
	NewURL string
}

func (e *GitRepositoryMovedError) Error() string {
	return fmt.Sprintf("repository moved to: %s", e.NewURL)
}
