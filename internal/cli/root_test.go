package cli

import (
	"bytes"
	"strings"
	"testing"
)

// P-01: with no subcommand there is no JSON result to print, so stdout must stay
// empty, the help must go to stderr and the command must fail (main exits 1).
func TestRootWithoutSubcommandKeepsStdoutEmpty(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("running hipoteca without a subcommand must fail")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Usage:") && !strings.Contains(stderr.String(), "hipoteca lee los tipos") {
		t.Errorf("the help must go to stderr, got %q", stderr.String())
	}
}

// The completion command is not part of the CLI contract, so asking for it must
// fail like any other unknown command instead of writing a shell script.
func TestCompletionCommandIsDisabled(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"completion", "bash"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("completion must not be a valid command")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
	}
}
