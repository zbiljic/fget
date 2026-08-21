package gitinspect

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner executes the Git commands used by repository inspection helpers.
type Runner interface {
	Run(ctx context.Context, dir string, args ...string) (Result, error)
}

// StreamingRunner can stream Git stdout without buffering large or binary
// output in memory.
type StreamingRunner interface {
	Runner
	RunTo(ctx context.Context, dir string, stdout io.Writer, args ...string) (Result, error)
}

// Result contains the output captured from a Git command.
type Result struct {
	Stdout string
	Stderr string
}

// CommandError describes a failed Git command without losing its captured output.
type CommandError struct {
	Args     []string
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func (e *CommandError) Error() string {
	if e == nil {
		return "git command failed"
	}
	message := strings.TrimSpace(e.Stderr)
	if message == "" {
		message = strings.TrimSpace(e.Stdout)
	}
	if message == "" && e.Err != nil {
		message = e.Err.Error()
	}
	if message == "" {
		message = "git command failed"
	}
	return message
}

func (e *CommandError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// CLIRunner invokes the Git CLI with optional repository locks disabled.
type CLIRunner struct{}

func (CLIRunner) Run(ctx context.Context, dir string, args ...string) (Result, error) {
	var stdout bytes.Buffer
	return runCLI(ctx, dir, &stdout, &stdout, args...)
}

func (CLIRunner) RunTo(ctx context.Context, dir string, stdout io.Writer, args ...string) (Result, error) {
	if stdout == nil {
		stdout = io.Discard
	}
	return runCLI(ctx, dir, stdout, nil, args...)
}

func runCLI(ctx context.Context, dir string, stdout io.Writer, capturedStdout *bytes.Buffer, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")

	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := Result{
		Stderr: stderr.String(),
	}
	if capturedStdout != nil {
		result.Stdout = capturedStdout.String()
	}
	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return result, &CommandError{
			Args:     append([]string(nil), args...),
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			ExitCode: exitCode,
			Err:      err,
		}
	}

	return result, nil
}
