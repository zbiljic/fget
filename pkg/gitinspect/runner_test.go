package gitinspect

import (
	"context"
	"errors"
	"testing"
)

func TestCLIRunnerReturnsCommandError(t *testing.T) {
	t.Parallel()

	_, err := (CLIRunner{}).Run(context.Background(), t.TempDir(), "definitely-not-a-git-subcommand")
	if err == nil {
		t.Fatal("CLIRunner.Run() error = nil")
	}
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("CLIRunner.Run() error = %T, want *CommandError", err)
	}
	if commandErr.ExitCode == 0 || len(commandErr.Args) != 1 {
		t.Fatalf("CommandError = %+v", commandErr)
	}
}
