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

## Contributing

Contributions are welcome!

1.  Fork the repository.
2.  Create a new feature branch.
3.  Make your changes.
4.  Ensure all checks pass by running `make pre-commit`.
5.  Submit a pull request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
