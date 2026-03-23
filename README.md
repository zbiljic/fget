# fget

[![Go Report Card](https://goreportcard.com/badge/github.com/zbiljic/fget)](https://goreportcard.com/report/github.com/zbiljic/fget)
[![Build Status](https://img.shields.io/github/actions/workflow/status/zbiljic/fget/golangci-lint.yml?branch=main)](https://github.com/zbiljic/fget/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**`fget` is a fast, concurrent, and opinionated CLI for managing all your local Git repositories.**

Do you have hundreds or thousands of repos cloned locally? Keeping them updated, fixing broken default branches, and cleaning up old objects is a chore. `fget` automates this with a set of powerful, concurrent commands.

## Features

-   **Fast & Concurrent:** `fget` performs operations on many repositories in parallel using a worker pool, making it significantly faster than sequential scripts.
-   **Smart `clone`:** Clones repositories into a clean, predictable `host/user/repo` directory structure.
-   **Powerful `update`:** Fetches updates for all repos in a directory tree. It's built to be resilient, with sophisticated retry logic for network issues.
-   **Resumable Progress:** Long-running commands like `update`, `fix`, and `gc` save their state. If you cancel the operation, you can run the same command again to resume where you left off.
-   **Automated `fix`:** This is the killer feature. `fget fix` automatically:
    -   Detects if a remote's default branch has changed (e.g., `master` to `main`) and updates your local clone to match.
    -   Resets your local branch to match the remote if it's behind, avoiding non-fast-forward errors.
    -   Cleans up dirty working directories.
    -   Repairs broken or invalid local references.
-   **Moved Repository Detection:** If a repository moves on the server (e.g., a user or organization rename on GitHub), `fget` detects the HTTP redirect and automatically renames your local directory and updates the remote URL.
-   **Efficient `gc`:** Run `git gc` concurrently across all your repositories to optimize their local storage.
-   **Safe `reclone`:** Re-clone repositories from scratch with an interactive confirmation prompt (or `--yes` to skip confirmation).
-   **Single Binary:** No dependencies, no runtime. Just a single executable file.

## Installation

### With `go`

The easiest way to install `fget` is using `go install`:

```sh
go install github.com/zbiljic/fget@latest
```

Ensure that your `GOBIN` is in your system's `PATH` (e.g., `export PATH=$PATH:$(go env GOBIN)`) to run `fget` directly from your terminal.

### Building from source

Alternatively, you can clone the repository and build `fget` yourself. This method requires `make`.

```bash
git clone https://github.com/zbiljic/fget.git
cd fget
make install
```

The `make install` command will compile the `fget` executable and place it in your `GOBIN` directory.

## Usage

`fget` operates on a root directory containing your projects. If you don't specify a path, it will use the current working directory.

### `clone`: Clone one or more repositories

`fget` will automatically create the directory structure based on the repository's URL.

```sh
# Clones into ./github.com/zbiljic/fget
fget clone https://github.com/zbiljic/fget.git
# or just
fget https://github.com/zbiljic/fget

# Clone multiple repos into a specific directory, e.g. ~/src
fget clone https://github.com/spf13/cobra https://github.com/pterm/pterm ~/src
# This will create:
# ~/src/github.com/spf13/cobra
# ~/src/github.com/pterm/pterm
```

### `update`: Update all repositories

This command (aliased as `up`) recursively finds all Git repositories under the target path and fetches the latest changes from their remotes.

```sh
# Update all repositories in ~/src using 16 parallel workers
fget update ~/src --workers 16

# Only print projects that were actually updated
fget up --only-updated
```

### `fix`: Fix inconsistencies

This is the most powerful command. It runs a series of checks and repairs on all your repositories.

```sh
# Inspect and fix all repositories under ~/src
fget fix ~/src

# Example: A repo's default branch was renamed from 'master' to 'main'
# `fget fix` will handle this automatically:
#
# [/] (active: 1)
# /home/user/src/github.com/some/repo
# ℹ github.com/some/repo
# └ main
# ! update HEAD 'main': success
```

### `gc`: Optimize repositories

This runs `git gc --prune=all` to clean up and optimize the local repositories.

```sh
# Run garbage collection on all repositories under ~/src
fget gc ~/src
```

### `reclone`: Re-clone repositories from scratch

This command first verifies remote access, then removes each provided local repository directory and clones it again from its configured `origin` URL.

```sh
# Interactive confirmation prompt before destructive action
fget reclone ~/src/github.com/zbiljic/fget

# Skip confirmation prompt (works in terminal and scripts)
fget reclone ~/src/github.com/zbiljic/fget ~/src/github.com/spf13/cobra --yes

# Alias
fget reset ~/src/github.com/zbiljic/fget --yes
```

### `list`: List all managed repositories

This command (aliased as `ls`) finds and prints the project identifiers for all local repositories.

```sh
fget list ~/src
# Output:
# github.com/zbiljic/fget
# github.com/spf13/cobra
# github.com/pterm/pterm
```

### `config`: Manage merged config, catalog, and tags

`fget` supports a merged configuration model and a machine-managed repository catalog:

- Global base config: `$XDG_CONFIG_HOME/fget/fget.yaml` or `~/.config/fget/fget.yaml`
- Overlay configs: `fget.yaml` files discovered from your home directory down to your current directory
- Catalog file: `$XDG_CONFIG_HOME/fget/catalog.yaml` or `~/.config/fget/catalog.yaml`

Example config file:

```yaml
version: "1"
roots:
  - ~/dev
catalog:
  path: "~/.config/fget/catalog.yaml"
```

Common commands:

```sh
# Create/update global config (~/.config/fget/fget.yaml by default)
fget config init

# Add specific roots to config (missing roots are merged in and sorted)
fget config init --root ~/dev --root ~/work

# Write local config in current directory
fget config init --local

# Write explicit config file (overwrites with minimal config when --force is set)
fget config init --file ~/tmp/fget.yaml --force

# Show effective merged configuration and resolved catalog path
fget config show

# Re-scan roots and update catalog (remove stale entries with --prune)
fget catalog sync --prune

# Override roots for one sync run
fget catalog sync --root ~/dev --root ~/work --prune

# Manage user-defined tags
fget tag add github.com/zbiljic/fget cli golang
fget tag remove github.com/zbiljic/fget cli
fget tag list
fget tag list github.com/zbiljic/fget

# Inspect the shared repository catalog
fget catalog list
fget catalog show github.com/zbiljic/fget
fget catalog paths github.com/zbiljic/fget github.com/cli/cli

# Create/update local link projection config in the current directory
fget link init fs___ --source-root ~/dev/src

# Sync a projection directory from catalog tags
cd ~/dev/wtopic___/fs___
fget link sync
```

`config init` creates or updates `fget.yaml`; `catalog sync` creates or refreshes `catalog.yaml`.

Projection directories can reuse the same `fget.yaml` format, or you can generate/update the
`link:` block with `fget link init <tag...>`:

```yaml
version: "1"

link:
  tags:
    - fs___
  match: any
  layout: repo-id
  root: .
  source_root: ~/dev/src
```

With the example above, any catalog repo tagged `fs___` is projected under the current directory using its repo ID as the relative path. For example, `github.com/cli/cli` becomes `./github.com/cli/cli`.

`fget link sync` is stateless:

- it reads the shared `catalog.yaml`
- it selects matching repos by tag
- it creates or updates symlinks under `link.root`
- it removes stale symlinks under that root
- it never removes real files or directories, so locally cloned repos can coexist with projected links

If a catalog repo has multiple locations, set `link.source_root` so `fget` can choose the correct clone path.

## Contributing

Contributions are welcome!

1.  Fork the repository.
2.  Create a new feature branch.
3.  Make your changes.
4.  Ensure all checks pass by running `make pre-commit`.
5.  Submit a pull request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
