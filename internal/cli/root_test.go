package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
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

// An unknown command is a usage error of the contract (§6.4): it must reach main
// as an errs error with kind usage (so the process exits 2), and stdout must stay
// empty because the envelope goes to stderr.
func TestUnknownCommandIsAUsageError(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"inventado"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("an unknown command must fail")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("the error must carry a contract kind, got %v", err)
	}
	if e.Kind != errs.KindUsage {
		t.Errorf("unknown command kind = %q, want usage", e.Kind)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
	}
}

// An unknown flag is also a usage error, with stdout empty.
func TestUnknownFlagIsAUsageError(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"version", "--bogus"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("an unknown flag must fail")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("the error must carry a contract kind, got %v", err)
	}
	if e.Kind != errs.KindUsage {
		t.Errorf("unknown flag kind = %q, want usage", e.Kind)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
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
