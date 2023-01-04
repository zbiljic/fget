package cmd

import (
	"errors"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	giturls "github.com/whilp/git-urls"
)

func gitProjectID(repo *git.Repository) (string, error) {
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
