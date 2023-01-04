package cmd

import (
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/imdario/mergo"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fsfind"
)

var fixCmd = &cobra.Command{
	Use:         "fix",
	Short:       "List local repositories",
	Annotations: map[string]string{"group": "update"},
	Args:        cobra.ArbitraryArgs,
	RunE:        runFix,
}

var fixCmdFlags = &fixOptions{}

func init() {
	fixCmd.Flags().BoolVar(&fixCmdFlags.DryRun, "dry-run", false, "Displays the operations that would be performed using the specified command without actually running them")

	rootCmd.AddCommand(fixCmd)
}

type fixOptions struct {
	Roots  []string
	DryRun bool
}

func runFix(cmd *cobra.Command, args []string) error {
	opts, err := parseFixArgs(args)
	if err != nil {
		return err
	}

	if err := mergo.Merge(&opts, fixCmdFlags); err != nil {
		return err
	}

	repoPaths, err := fsfind.GitDirectoriesTree(opts.Roots...)
	if err != nil {
		return err
	}

	for it := repoPaths.Iterator(); it.HasNext(); {
		node, _ := it.Next()
		repoPath := string(node.Key())

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
				project, err := gitProjectID(repo)
				if err != nil {
					return err
				}

				printInfoNoEndline(os.Stdout, "%s", project)
				printInfoNoEndline(os.Stdout, " [")
				printWarningNoEndline(os.Stdout, "'%s': reference broken", ref.Name())
				printInfoNoEndline(os.Stdout, "]: ")

				if opts.DryRun {
					printSuccess(os.Stdout, "dry-run")
				} else {
					err := repo.Storer.RemoveReference(ref.Name())
					if err != nil {
						printErr(os.Stdout, err)
					} else {
						printSuccess(os.Stdout, "success")
					}
				}
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func parseFixArgs(args []string) (fixOptions, error) {
	opts := fixOptions{}

	if len(args) > 0 {
		for _, arg := range args {
			path, err := fsfind.DirAbsPath(arg)
			if err != nil {
				return opts, err
			}

			opts.Roots = append(opts.Roots, path)
		}
	} else {
		// fallback to current working directory
		opts.Roots = append(opts.Roots, GetWd())
	}

	return opts, nil
}
